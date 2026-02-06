require('dotenv').config();

const fs = require('fs');
const path = require('path');
const axios = require('axios');

/* ================= CONFIG ================= */

const AUTH_API_ENDPOINT = process.env.AUTH_API_ENDPOINT;
const CLIENT_ID = process.env.CLIENT_ID;
const CLIENT_SECRET = process.env.CLIENT_SECRET;

let authToken = null;
let tokenExpiresAt = 0;
let restInstanceUrl = null;

/* ================= AUTH ================= */

async function authenticate() {
  const currentTime = Math.floor(Date.now() / 1000);

  if (authToken && tokenExpiresAt > currentTime + 60) {
    return authToken;
  }

  console.log('Requesting new auth token...');

  const response = await axios.post(
    AUTH_API_ENDPOINT,
    {
      grant_type: 'client_credentials',
      client_id: CLIENT_ID,
      client_secret: CLIENT_SECRET
    },
    {
      headers: { 'Content-Type': 'application/json' }
    }
  );

  authToken = response.data.access_token;
  tokenExpiresAt = currentTime + response.data.expires_in;
  restInstanceUrl = response.data.rest_instance_url;

  console.log('Auth success');
  console.log('REST instance:', restInstanceUrl);

  return authToken;
}

/* ================= ENDPOINT ================= */

function getDeleteContactEndpoint() {
  if (!restInstanceUrl) {
    throw new Error('REST instance URL not available. Authenticate first.');
  }

  const baseUrl = restInstanceUrl.replace(/\/$/, '');
  return `${baseUrl}/contacts/v1/contacts/actions/delete?type=keys`;
}

/* ================= DELETE CONTACT ================= */

async function deleteContact(email) {
  const token = await authenticate();
  const endpoint = getDeleteContactEndpoint();

  const payload = {
    values: [
      {
        key: email,
        contactKey: email
      }
    ]
  };

  const response = await axios.post(endpoint, payload, {
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json'
    }
  });

  return response.data;
}

/* ================= BATCH DELETE ================= */

async function deleteContactsInBatches(contacts, batchSize = 100) {
  const token = await authenticate();
  const endpoint = getDeleteContactEndpoint();

  for (let i = 0; i < contacts.length; i += batchSize) {
    const batch = contacts.slice(i, i + batchSize);

    console.log(
      `Deleting batch ${i / batchSize + 1} (${batch.length} contacts)`
    );

    const payload = {
      values: batch.map(email => email)
    };

    try {
      const response = await axios.post(endpoint, payload, {
        headers: {
          Authorization: `Bearer ${token}`,
          'Content-Type': 'application/json'
        }
      });

      console.log('Batch deleted:', response.data);
    } catch (err) {
      console.error(
        'Batch delete failed:',
        err.response?.data || err.message
      );
    }
  }
}

/* ================= READ JSON FILE ================= */

function readContactsFromFile(filePath) {
  const raw = fs.readFileSync(filePath, 'utf8');
  const json = JSON.parse(raw);

  if (!Array.isArray(json.contacts)) {
    throw new Error('contacts field not found or invalid');
  }

  return json.contacts;
}

/* ================= MAIN ================= */

(async () => {
  try {
    const filePath = path.join(
      __dirname,
      'contacts.json'
    );

    const contacts = readContactsFromFile(filePath);

    console.log(`Loaded ${contacts.length} contacts`);

    await deleteContactsInBatches(contacts, 50);
  } catch (err) {
    console.error('Error:', err.message);
  }
})();
