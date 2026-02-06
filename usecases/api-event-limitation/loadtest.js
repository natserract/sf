import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';
import { randomString, randomItem } from 'https://jslib.k6.io/k6-utils/1.2.0/index.js';


// Custom Metrics
const apiErrors = new Counter('api_errors');
const apiSuccess = new Counter('api_success');
const responseTimes = new Trend('response_times');
const limitViolations = new Counter('limit_violations');
const authErrors = new Counter('auth_errors');

// Configuration
const AUTH_API_ENDPOINT = __ENV.AUTH_API_ENDPOINT;
const CLIENT_ID = __ENV.CLIENT_ID;
const CLIENT_SECRET = __ENV.CLIENT_SECRET;
const EVENT_DEFINITION_KEY = 'APIEvent-33b86a60-9379-f8e3-599d-a559c67640a0';

// Global auth state (shared across VUs)
let authToken = null;
let tokenExpiresAt = 0;
let restInstanceUrl = null;

let globalContactsInTest = []
let createdContacts = [];

// Test data generators
const firstNames = ['Alfin', 'Budi', 'Citra', 'Dewi', 'Eko', 'Farah', 'Gita', 'Hadi', 'Indra', 'Joko',
    'Kartika', 'Lina', 'Made', 'Nur', 'Omar', 'Putri', 'Qori', 'Rini', 'Sari', 'Tono',
    'Usman', 'Vina', 'Wati', 'Xena', 'Yudi', 'Zara'];

const lastNames = ['Surya', 'Pratama', 'Santoso', 'Wijaya', 'Kusuma', 'Permata', 'Saputra', 'Rahman',
    'Hidayat', 'Nugroho', 'Lestari', 'Saptono', 'Wibowo', 'Firmansyah', 'Setiawan',
    'Andriani', 'Maulana', 'Rizki', 'Purnomo', 'Utami'];

const tierNames = ['Crew', 'Silver', 'Gold', 'Platinum', 'Diamond'];

const emailDomains = ['gmail.com', 'yahoo.com', 'hotmail.com', 'outlook.com', 'mail.com'];

// Authentication function
function authenticate() {
    const currentTime = Math.floor(Date.now() / 1000);

    // Check if token is still valid (with 60 second buffer)
    if (authToken && tokenExpiresAt > currentTime + 60) {
        console.log(`Using existing token (expires in ${tokenExpiresAt - currentTime} seconds)`);
        return authToken;
    }

    console.log('Requesting new authentication token...');

    const authPayload = {
        grant_type: 'client_credentials',
        client_id: CLIENT_ID,
        client_secret: CLIENT_SECRET
    };

    const authParams = {
        headers: {
            'Content-Type': 'application/json'
        }
    };

    const authResponse = http.post(AUTH_API_ENDPOINT, JSON.stringify(authPayload), authParams);

    const authSuccess = check(authResponse, {
        'auth status is 200': (r) => r.status === 200,
        'auth has access_token': (r) => {
            try {
                const body = JSON.parse(r.body);
                return body.access_token !== undefined;
            } catch (e) {
                return false;
            }
        }
    });

    if (!authSuccess) {
        authErrors.add(1);
        console.error(`Authentication failed! Status: ${authResponse.status}, Body: ${authResponse.body}`);
        throw new Error('Authentication failed');
    }

    const authData = JSON.parse(authResponse.body);

    // Store auth data
    authToken = authData.access_token;
    tokenExpiresAt = currentTime + authData.expires_in;
    restInstanceUrl = authData.rest_instance_url;

    console.log(`Authentication successful! Token expires in ${authData.expires_in} seconds`);
    console.log(`REST Instance URL: ${restInstanceUrl}`);

    return authToken;
}

// Get API endpoint with instance URL
function getEventApiEndpoint() {
    if (!restInstanceUrl) {
        throw new Error('REST instance URL not available. Authenticate first.');
    }
    return `${restInstanceUrl}interaction/v1/events`;
}

// Generate random contact data
function generateContact() {
    const firstName = randomItem(firstNames);
    const lastName = randomItem(lastNames);
    const randomNum = Math.floor(Math.random() * 100000);
    const domain = randomItem(emailDomains);
    const email = `${firstName.toLowerCase()}${lastName.toLowerCase()}${randomNum}@${domain}`;

    return {
        contactKey: email,
        email: email,
        fullName: `${firstName} ${lastName}`,
        tierName: randomItem(tierNames),
        pointsBalance: (Math.random() * 1000).toFixed(2)
    };
}

// Create API event payload
function createEventPayload(contact) {
    return {
        ContactKey: contact.contactKey,
        EventDefinitionKey: EVENT_DEFINITION_KEY,
        Data: {
            ContactKey: contact.contactKey,
            Email: contact.email,
            FullName: contact.fullName,
            TierName: contact.tierName,
            PointsBalance: contact.pointsBalance
        }
    };
}

function createBatchEventPayload(contacts) {
    return {
        contacts: contacts.map(contact => ({
            keys: {
                contactKey: contact.contactKey
            },
            attributeSets: [{
                name: "LoyaltyMember",
                items: [{
                    values: {
                        email: contact.email,
                        fullName: contact.fullName,
                        tierName: contact.tierName,
                        pointsBalance: contact.pointsBalance
                    }
                }]
            }]
        }))
    };
}

function createEventPayloadWithMultipleRecords(contacts) {
    return {
        ContactKey: contacts.map(c => c.contactKey),
        EventDefinitionKey: EVENT_DEFINITION_KEY,
        Data: contacts.map(c => ({
            FirstName: c.firstName,
            LastName: c.lastName,
            Email: c.email,
            // ... other fields
        }))
    };
}

// Send single event
function sendEvent(payload, testCase, batchId, sequenceNum) {
    // Ensure we have valid auth token
    const token = authenticate();
    const apiEndpoint = getEventApiEndpoint();

    const startTime = Date.now();

    const params = {
        headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`
        },
        tags: {
            test_case: testCase,
            batch_id: batchId,
            sequence: sequenceNum
        }
    };

    const response = http.post(apiEndpoint, JSON.stringify(payload), params);
    const duration = Date.now() - startTime;

    // Record metrics
    responseTimes.add(duration);

    const success = check(response, {
        'status is 200': (r) => r.status === 200,
        'response has body': (r) => r.body.length > 0,
    });

    if (success) {
        apiSuccess.add(1);
        createdContacts.push(payload.ContactKey);
    } else {
        apiErrors.add(1);

        // Check for auth errors (token expired mid-test)
        if (response.status === 401) {
            console.log('Auth token expired, forcing re-authentication...');
            authToken = null; // Force new auth
            tokenExpiresAt = 0;
        }

        // Check for limit violations
        if (response.status === 429 ||
            (response.body && response.body.includes('limit')) ||
            (response.body && response.body.includes('exceeded'))) {
            limitViolations.add(1);
        }

        console.log(`ERROR [${testCase}] Seq:${sequenceNum} - Status: ${response.status}, Body: ${response.body}`);
    }

    // Log to monitoring store
    const logEntry = {
        test_run_id: __ENV.TEST_RUN_ID || 'test-' + Date.now(),
        test_case_name: testCase,
        batch_id: batchId,
        event_sequence_number: sequenceNum,
        request_sent_time: new Date(startTime).toISOString(),
        response_received_time: new Date().toISOString(),
        response_time_ms: duration,
        http_status_code: response.status,
        success_flag: success,
        contact_key: payload.ContactKey,
        response_body: response.body,
        error_message: success ? null : response.body
    };

    // You can send this to your monitoring database
    // Example: http.post('YOUR_MONITORING_ENDPOINT', JSON.stringify(logEntry));

    return { response, duration, success };
}

// Send batch of events with authentication
function sendBatchWithAuth(requests, testCase, batchId) {
    // Ensure we have valid auth token
    const token = authenticate();
    const apiEndpoint = getEventApiEndpoint();

    const responses = http.batch(
        requests.map((req) => ({
            method: 'POST',
            url: apiEndpoint,
            body: JSON.stringify(req.payload),
            params: {
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': `Bearer ${token}`
                },
                tags: {
                    test_case: testCase,
                    batch_id: batchId,
                    sequence: req.sequence
                }
            }
        }))
    );

    return responses;
}

// Get Delete Contact API endpoint
function getDeleteContactEndpoint() {
    if (!restInstanceUrl) {
        throw new Error('REST instance URL not available. Authenticate first.');
    }
    const baseUrl = restInstanceUrl.replace(/\/$/, '');
    return `${baseUrl}/contacts/v1/contacts/actions/delete?type=keys`;
}

function deleteContact(contactKeys) {
    // Ensure we have valid auth token
    const token = authenticate();
    const deleteEndpoint = getDeleteContactEndpoint();

    // Convert single contact key to array
    const keysArray = Array.isArray(contactKeys) ? contactKeys : [contactKeys];

    console.log(`Deleting ${keysArray.length} contact(s)...`);

    const deletePayload = {
        values: keysArray,
        DeleteOperationType: "ContactAndAttributes"
    };

    const startTime = Date.now();

    const params = {
        headers: {
            'Content-Type': 'application/json',
            'Authorization': `Bearer ${token}`
        }
    };

    const response = http.post(deleteEndpoint, JSON.stringify(deletePayload), params);
    const duration = Date.now() - startTime;

    const success = check(response, {
        'delete status is 200 or 202': (r) => r.status === 200 || r.status === 202,
        'delete response has body': (r) => r.body.length > 0
    });

    if (success) {
        deleteSuccess.add(keysArray.length);
        console.log(`✓ Successfully deleted ${keysArray.length} contact(s) in ${duration}ms`);
    } else {
        deleteErrors.add(keysArray.length);
        console.error(`✗ Delete failed - Status: ${response.status}, Body: ${response.body}`);

        // Handle auth errors
        if (response.status === 401) {
            console.log('Auth token expired during delete, forcing re-authentication...');
            authToken = null;
            tokenExpiresAt = 0;
        }
    }

    return {
        success: success,
        statusCode: response.status,
        responseBody: response.body,
        responseTimeMs: duration,
        deletedCount: keysArray.length,
        contactKeys: keysArray,
        timestamp: new Date().toISOString()
    };
}

function deleteContactsInBatches(contactKeys, batchSize = 50) {
    console.log(`\n=== Deleting ${contactKeys.length} contacts in batches of ${batchSize} ===`);

    let totalDeleted = 0;
    let totalFailed = 0;

    for (let i = 0; i < contactKeys.length; i += batchSize) {
        const batch = contactKeys.slice(i, i + batchSize);
        const batchNum = Math.floor(i / batchSize) + 1;
        const totalBatches = Math.ceil(contactKeys.length / batchSize);

        console.log(`Processing batch ${batchNum}/${totalBatches} (${batch.length} contacts)`);

        const result = deleteContact(batch);

        if (result.success) {
            totalDeleted += batch.length;
        } else {
            totalFailed += batch.length;
        }

        // Small delay between batches to avoid rate limiting
        if (i + batchSize < contactKeys.length) {
            sleep(1);
        }
    }

    console.log(`\nDeletion Summary:`);
    console.log(`  Total Deleted: ${totalDeleted}`);
    console.log(`  Total Failed: ${totalFailed}`);
    console.log(`  Total Processed: ${contactKeys.length}\n`);

    return {
        totalDeleted: totalDeleted,
        totalFailed: totalFailed,
        totalProcessed: contactKeys.length
    };
}

// Test Case 1: 60 events simultaneously (Exceed 50 records per transaction)
export function testCase1_60EventsSimultaneous() {
    const testCase = 'Records_Per_Transaction_Over_Limit';
    const batchId = `batch-tc1-${Date.now()}`;
    const contactsInTest = [];

    console.log(`\n=== TEST CASE 1: Sending 60 events simultaneously ===`);
    console.log(`Objective: Exceed 50 records per transaction limit`);
    console.log(`Expected: 60 events * 1 record = 60 total records (Exceeds limit)\n`);

    const requests = [];

    // Prepare 60 events
    for (let i = 1; i <= 60; i++) {
        const contact = generateContact();
        const payload = createEventPayload(contact);
        requests.push({ payload, sequence: i });
        contactsInTest.push(contact.contactKey);
    }

    // Write to file using Node.js
    const contactsData = {
        testCase: testCase,
        batchId: batchId,
        timestamp: new Date().toISOString(),
        contacts: contactsInTest
    };

    // Send all 60 simultaneously
    const responses = sendBatchWithAuth(requests, testCase, batchId);

    // Analyze results
    let successCount = 0;
    let failCount = 0;

    responses.forEach((response, idx) => {
        if (response.status === 200 || response.status == 201) {
            successCount++;
            apiSuccess.add(1);
            createdContacts.push(contactsInTest[idx]);
        } else {
            failCount++;
            apiErrors.add(1);
            limitViolations.add(1);
        }
        responseTimes.add(response.timings.duration);
    });

    console.log(`Results: ${successCount} success, ${failCount} failed`);
    console.log(`Expected behavior: First 50 succeed, remaining 10 fail/queue\n`);

    sleep(2);
}

// Test Case 2: 50 events, each creates 5 records (Exceed 200 record processing limit)
export function testCase2_50EventsWith5Records() {
    const testCase = 'Record_Processing_Over_Limit';
    const batchId = `batch-tc2-${Date.now()}`;
    const contactsInTest = [];

    console.log(`\n=== TEST CASE 2: Sending 50 events (each creates 5 records) ===`);
    console.log(`Objective: Exceed 200 record processing limit`);
    console.log(`Expected: 50 events * 5 records = 250 total records (Exceeds 200 limit)\n`);

    const requests = [];

    // Create 10 groups, each with 5 events (total 50 events)
    for (let group = 1; group <= 10; group++) {
        // Each group has 5 events/records
        for (let record = 1; record <= 5; record++) {
            const contact = generateContact();
            const payload = {
                ContactKey: contact.contactKey,
                EventDefinitionKey: EVENT_DEFINITION_KEY,
                Data: {
                    ContactKey: contact.contactKey,
                    Email: contact.email,
                    FullName: contact.fullName,
                    TierName: contact.tierName,
                    PointsBalance: contact.pointsBalance
                }
            };

            const sequenceNumber = (group - 1) * 5 + record;
            requests.push({
                payload,
                sequence: sequenceNumber,
                group: group,
                recordInGroup: record
            });
            contactsInTest.push(contact.contactKey);
        }
    }

    console.log(`Prepared ${requests.length} events in ${requests.length / 5} groups`);

    // Send all 50 events simultaneously in one batch
    const responses = sendBatchWithAuth(requests, testCase, batchId);

    // Analyze results
    let successCount = 0;
    let failCount = 0;
    const groupResults = {};

    responses.forEach((response, idx) => {
        const groupNum = requests[idx].group;
        console.log('response', response, 'groupNum: ', groupNum)


        if (!groupResults[groupNum]) {
            groupResults[groupNum] = { success: 0, failed: 0 };
        }

        if (response.status === 200 || response.status === 201) {
            successCount++;
            apiSuccess.add(1);
            createdContacts.push(contactsInTest[idx]);
            groupResults[groupNum].success++;
        } else {
            failCount++;
            apiErrors.add(1);
            limitViolations.add(1);
            groupResults[groupNum].failed++;
        }
        responseTimes.add(response.timings.duration);
    });

    console.log(`\nResults: ${successCount} success, ${failCount} failed`);
    console.log(`Group breakdown:`);
    Object.keys(groupResults).forEach(group => {
        const result = groupResults[group];
        console.log(`  Group ${group}: ${result.success} success, ${result.failed} failed`);
    });
    console.log(`Expected behavior: All 50 events should succeed if within rate limits\n`);

    sleep(2);
}

// Test Case 3: 2,500 events within 10 minutes (Exceed 2,000 flow invocations)
export function testCase3_2500EventsIn10Minutes() {
    const testCase = 'Flow_Invocations_Over_Limit';
    const batchId = `batch-tc3-${Date.now()}`;
    const contactsInTest = [];

    console.log(`\n=== TEST CASE 3: Sending 2,500 events within 10 minutes ===`);
    console.log(`Objective: Exceed 2,000 flow invocations`);
    console.log(`Strategy: Send in batches of 50 events, with delays between batches\n`);

    const totalEvents = 2500;
    const batchSize = 50;
    const numBatches = totalEvents / batchSize; // 50 batches
    const durationSeconds = 600; // 10 minutes
    const delayBetweenBatches = durationSeconds / numBatches; // ~12 seconds between batches

    console.log(`Sending ${numBatches} batches of ${batchSize} events`);
    console.log(`Delay between batches: ${delayBetweenBatches.toFixed(2)} seconds\n`);

    let totalSuccess = 0;
    let totalFailed = 0;
    const startTime = Date.now();

    for (let batch = 1; batch <= numBatches; batch++) {
        const requests = [];
        const batchContacts = [];

        // Prepare batch of events
        for (let i = 1; i <= batchSize; i++) {
            const contact = generateContact();
            const payload = {
                ContactKey: contact.contactKey,
                EventDefinitionKey: EVENT_DEFINITION_KEY,
                Data: {
                    ContactKey: contact.contactKey,
                    Email: contact.email,
                    FullName: contact.fullName,
                    TierName: contact.tierName,
                    PointsBalance: contact.pointsBalance
                }
            };

            const eventNumber = (batch - 1) * batchSize + i;
            requests.push({ 
                payload, 
                sequence: eventNumber,
                batch: batch
            });
            batchContacts.push(contact.contactKey);
            contactsInTest.push(contact.contactKey);
        }

        // Send batch simultaneously
        const responses = sendBatchWithAuth(requests, testCase, `${batchId}-batch${batch}`);

        // Analyze batch results
        let batchSuccess = 0;
        let batchFailed = 0;

        responses.forEach((response, idx) => {
            if (response.status === 200 || response.status === 201) {
                batchSuccess++;
                apiSuccess.add(1);
                createdContacts.push(batchContacts[idx]);
            } else {
                batchFailed++;
                apiErrors.add(1);
                limitViolations.add(1);
            }
            responseTimes.add(response.timings.duration);
        });

        totalSuccess += batchSuccess;
        totalFailed += batchFailed;

        // Progress indicator
        const elapsedSeconds = (Date.now() - startTime) / 1000;
        const eventsProcessed = batch * batchSize;
        console.log(`Batch ${batch}/${numBatches}: ${batchSuccess} success, ${batchFailed} failed | Total: ${eventsProcessed}/${totalEvents} events | Elapsed: ${elapsedSeconds.toFixed(1)}s`);

        // Sleep between batches (except after last batch)
        if (batch < numBatches) {
            sleep(delayBetweenBatches);
        }
    }

    const totalElapsed = (Date.now() - startTime) / 1000;
    console.log(`\n=== Test Case 3 Summary ===`);
    console.log(`Total events: ${totalEvents}`);
    console.log(`Successful: ${totalSuccess}`);
    console.log(`Failed: ${totalFailed}`);
    console.log(`Total time: ${totalElapsed.toFixed(1)}s (target: ${durationSeconds}s)`);
    console.log(`Average rate: ${(totalEvents / totalElapsed * 60).toFixed(1)} events/minute`);
    console.log(`Expected: First ~2000 succeed, remaining ~500 fail/throttle\n`);

    sleep(2);
}

// Test Case 4: 60 events, each qualifies for 2 journeys (Exceed 50 enqueue jobs)
export function testCase4_60EventsFor2Journeys() {
    const testCase = 'Enqueue_Jobs_Over_Limit';
    const batchId = `batch-tc4-${Date.now()}`;

    console.log(`\n=== TEST CASE 4: Sending 60 events (each qualifies for 2 journeys) ===`);
    console.log(`Objective: Exceed 50 enqueue jobs per transaction`);
    console.log(`Expected: 60 events * 2 journeys = 120 jobs (Exceeds 50 limit)\n`);

    // Use specific tier names that trigger multiple journeys
    const multiJourneyTiers = ['Gold', 'Platinum']; // Assume these trigger 2 journeys each

    const requests = [];

    for (let i = 1; i <= 60; i++) {
        const contact = generateContact();
        // Override tier to ensure it qualifies for multiple journeys
        contact.tierName = randomItem(multiJourneyTiers);

        const payload = createEventPayload(contact);
        requests.push({ payload, sequence: i });
    }

    // Send all 60 simultaneously
    const responses = sendBatchWithAuth(requests, testCase, batchId);

    // Analyze results
    let successCount = 0;
    let failCount = 0;

    responses.forEach((response) => {
        if (response.status === 200) {
            successCount++;
            apiSuccess.add(1);
        } else {
            failCount++;
            apiErrors.add(1);
            limitViolations.add(1);

            if (response.body.includes('queueable') || response.body.includes('enqueue')) {
                console.log(`Enqueue limit hit: ${response.body}`);
            }
        }
        responseTimes.add(response.timings.duration);
    });

    console.log(`Results: ${successCount} success, ${failCount} failed`);
    console.log(`Expected: Failures after 50 enqueue jobs are consumed\n`);

    sleep(2);
}

// Setup function - runs once per VU
export function setup() {
    console.log('\n=== INITIALIZING LOAD TEST ===');
    console.log('Authenticating...\n');

    // Initial authentication
    authenticate();

    return {
        testRunId: __ENV.TEST_RUN_ID || 'test-run-' + Date.now(),
        startTime: new Date().toISOString()
    };
}

// Main execution options
export const options = {
    scenarios: {
        test_case_1: {
            executor: 'shared-iterations',
            vus: 3,
            iterations: 3,
            exec: 'testCase1_60EventsSimultaneous',
        },
        test_case_2: {
            executor: 'shared-iterations',
            vus: 3,  // 3 users sending batches at the same time
            iterations: 3, // Each VU sends 1 batch (total 150 events)
            exec: 'testCase2_50EventsWith5Records',
        },
        test_case_3: {
            executor: 'shared-iterations',
            vus: 1,
            iterations: 1,
            exec: 'testCase3_2500EventsIn10Minutes',
            startTime: '0s',
            gracefulStop: '30s', // Allow last batch to complete
        },
        test_case_4: {
            executor: 'shared-iterations',
            vus: 1,
            iterations: 1,
            exec: 'testCase4_60EventsFor2Journeys',
            startTime: '720s', // Start after test case 3 (10 min + buffer)
        },
    },
    thresholds: {
        'http_req_duration': ['p(95)<5000'], // 95% of requests should be below 5s
        'api_errors': ['count<100'], // Less than 100 total errors (adjust as needed)
        // 'limit_violations': ['count>0'], // We EXPECT limit violations in these tests
        'auth_errors': ['count<5'], // Authentication should be reliable
    },
};

// Summary handler
export function handleSummary(data) {
    console.log('handleSummary called!'); // Debug log
    // console.log('createdContacts:', createdContacts);

    return {
        'summary.json': JSON.stringify(data),
        // 'contacts.json': JSON.stringify(createdContacts, null, 2),
        stdout: textSummary(data, { indent: ' ', enableColors: true }),
    };
}

// Helper for text summary
import { textSummary } from 'https://jslib.k6.io/k6-summary/0.0.1/index.js';