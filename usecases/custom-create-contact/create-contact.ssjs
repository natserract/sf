<script runat="server">
    /**
     * Custom Create Contact API
     *
     * Create or update a contact AND assign subscription status (Subscribed / Unsubscribed) in a
     * single unified flow based on user consent
     **/
    Platform.Load("core", "1");

    // Config
    var BASE_URL = "https://{domain}.rest.marketingcloudapis.com";
    var AUTH_BASE_URL = "https://{domain}.auth.marketingcloudapis.com";

    var ERROR_LOG_DE = "Loyalty_Create_Contact_Error_Logs";
    var RETRY_MAX_ATTEMPTS = 3; // Retry config for transient API failures (429, 5xx, network/timeout).
    var RETRY_BACKOFF_MS = 1000;

    var ClientID, ClientSecret, AccountID;
    ClientID = "{client_id}";
    ClientSecret = "{client_secret}";
    AccountID = "{mid}";

    // Auth token cache (Data Extension). In SFMC, set DE Customer Key (External Key) to exactly "Loyalty_AuthToken_Cache".
    // Columns: CacheKey (Text, Primary Key), AccessToken (Text 2000), ExpiresAt (Number).
    var AUTH_TOKEN_CACHE_DE = "Loyalty_AuthToken_Cache";
    var AUTH_TOKEN_CACHE_KEY = "default" + "_" + ClientID + "_" + AccountID;
    var AUTH_TOKEN_EXPIRY_BUFFER_SEC = 120;

    ///////// Main API Handler
    try {
        var requestData;
        var postData = Platform.Request.GetPostData(); // Try to get POST data first

        // Get Auth Token (with retry for transient failures)
        var accessTokenRes = withRetry(getAuthToken, isRetryableAuth);
        if (!accessTokenRes.result.valid) {
            returnErrorAndLog(
                500,
                {
                    success: false,
                    error: accessTokenRes.result.reason,
                    attempts: accessTokenRes.attempts
                },
                "Auth",
                "",
                "",
                "",
                postData || ""
            );
            return;
        }

        var accessToken = accessTokenRes.result.token;

        // Ensure payload doesn't empty
        if (postData && postData !== "") {
            // POST request
            try {
                requestData = Platform.Function.ParseJSON(postData);
            } catch (ex) {
                returnErrorAndLog(
                    400,
                    "Invalid JSON in request body",
                    "Validation",
                    ex && ex.stack ? ex.stack : "",
                    "",
                    "",
                    postData || ""
                );
                return;
            }
        } else {
            returnErrorAndLog(
                400,
                "Payload was empty",
                "Validation",
                "",
                "",
                "",
                postData || ""
            );
            return;
        }

        // Validate required fields
        // Required: contact_key, external_id,
        if (!requestData.contact_key) {
            var emailOrPhone =
                requestData && requestData.email
                    ? requestData.email
                    : requestData.phone
                      ? requestData.phone
                      : "";
            returnErrorAndLog(
                400,
                "Missing required field: contact_key",
                "Validation",
                "",
                emailOrPhone,
                emailOrPhone,
                postData || ""
            );
            return;
        }
        if (!requestData.external_id) {
            var emailOrPhone =
                requestData && requestData.email
                    ? requestData.email
                    : requestData.phone
                      ? requestData.phone
                      : "";
            returnErrorAndLog(
                400,
                "Missing required field: external_id",
                "Validation",
                "",
                emailOrPhone,
                emailOrPhone,
                postData || ""
            );
            return;
        }

        // Check if email or phone should exists
        if (!requestData.phone && !requestData.email) {
            var identifier =
                requestData && requestData.external_id
                    ? requestData.external_id
                    : "";

            returnErrorAndLog(
                400,
                "Missing required field: phone or email should exists",
                "Validation",
                "",
                identifier,
                identifier,
                postData || ""
            );
            return;
        }

        var countryCode = "";

        // Detect phone number country
        if (requestData.phone) {
            var locale = detectCountryFromPhone(requestData.phone);
            if (!locale) {
                var emailOrPhone =
                    requestData && requestData.email
                        ? requestData.email
                        : requestData.phone
                          ? requestData.phone
                          : "";

                returnErrorAndLog(
                    400,
                    "Invalid: phone number. Country with this phone number not found",
                    "Validation",
                    "",
                    emailOrPhone,
                    emailOrPhone,
                    postData || ""
                );
                return;
            }

            countryCode = locale ? locale.iso2 : "";
        }

        // Trigger Create Contact
        var createContactResult = withRetry(function () {
            return createContact(
                requestData.external_id,
                {
                    email: requestData.email,
                    externalId: requestData.external_id,
                    mobileNumber: requestData.phone || "",
                    locale: countryCode,
                    firstName: requestData.first_name,
                    lastName: requestData.last_name,
                    whatsappConsent: requestData.is_whatsapp_consent,
                    emailConsent: requestData.is_email_consent
                },
                accessToken
            );
        }, isRetryableApi);
        if (!createContactResult.result.valid) {
            returnErrorAndLog(
                500,
                {
                    success: false,
                    error: createContactResult.result.reason,
                    attempts: createContactResult.attempts
                },
                "CreateContact",
                "",
                requestData.email || requestData.phone,
                requestData.email || requestData.phone,
                postData || ""
            );
            return;
        }

        var successPayload = {
            success: true,
            message: "Contact succesfully created",
            contactKey: createContactResult.result.contactKey
        };
        returnSuccess(200, successPayload);
    } catch (ex) {
        var serverErrorEmail =
            typeof requestData !== "undefined" &&
            requestData &&
            requestData.Data &&
            requestData.Data.email
                ? requestData.Data.email
                : "";
        returnErrorAndLog(
            500,
            "Server error: " + ex.toString(),
            "Server",
            ex && ex.stack ? ex.stack : "",
            serverErrorEmail,
            serverErrorEmail,
            postData || ""
        );
    }

    ///////// API FUNCTIONS

    function getAuthToken() {
        var nowSec = Math.floor(new Date().getTime() / 1000);
        var minExpiresAt = nowSec + AUTH_TOKEN_EXPIRY_BUFFER_SEC;

        // Try cache read (fallback to auth request on any cache error)
        try {
            var cacheRows = Platform.Function.LookupRows(
                AUTH_TOKEN_CACHE_DE,
                "CacheKey",
                AUTH_TOKEN_CACHE_KEY
            );
            if (cacheRows && cacheRows.length > 0) {
                var row = cacheRows[0];
                var cachedToken = row.AccessToken;
                var expiresAt = Number(row.ExpiresAt);
                if (cachedToken && expiresAt > minExpiresAt) {
                    return { valid: true, token: String(cachedToken) };
                }
            }
        } catch (cacheEx) {
            // Continue to auth request
        }

        // Cache miss or expired: request new token
        try {
            var authEndpoint = AUTH_BASE_URL + "/v2/token";
            var payload = {
                grant_type: "client_credentials",
                client_id: ClientID,
                client_secret: ClientSecret
            };
            if (AccountID) {
                payload.account_id = AccountID;
            }

            var result = HTTP.Post(
                authEndpoint,
                "application/json",
                Stringify(payload),
                [],
                []
            );

            var parsedResponse = null;
            if (result.Response && result.Response[0]) {
                try {
                    parsedResponse = Platform.Function.ParseJSON(
                        result.Response[0]
                    );
                } catch (parseEx) {
                    parsedResponse = null;
                }
            }

            if (result.StatusCode == 200 && parsedResponse) {
                var token = String(parsedResponse.access_token);
                var expiresIn = Number(parsedResponse.expires_in) || 0;
                var expiresAt = nowSec + expiresIn - AUTH_TOKEN_EXPIRY_BUFFER_SEC;

                try {
                    Platform.Function.InsertData(
                        AUTH_TOKEN_CACHE_DE,
                        ["CacheKey", "AccessToken", "ExpiresAt"],
                        [AUTH_TOKEN_CACHE_KEY, token, expiresAt]
                    );
                } catch (insertEx) {
                    // Row may already exist (e.g. duplicate key); try update
                    try {
                        Platform.Function.UpdateData(
                            AUTH_TOKEN_CACHE_DE,
                            ["CacheKey"],
                            [AUTH_TOKEN_CACHE_KEY],
                            ["AccessToken", "ExpiresAt"],
                            [token, expiresAt]
                        );
                    } catch (updateEx) {
                        throw new Error(
                            "Auth token cache write failed. Insert: " +
                                insertEx.toString() +
                                "; Update: " +
                                updateEx.toString()
                        );
                    }
                }

                return { valid: true, token: token };
            }

            var reason = parsedResponse
                ? Stringify(parsedResponse)
                : result.Response && result.Response[0]
                  ? String(result.Response[0])
                  : "Unknown auth error";
            return {
                valid: false,
                reason: "Error: " + reason,
                statusCode: result.StatusCode
            };
        } catch (ex) {
            return {
                valid: false,
                reason: ex.toString()
            };
        }
    }

    /**
     * Creates a new contact in Salesforce Marketing Cloud via the Contacts REST API.
     *
     * @param  {object} contactKey         Unique contact key (e.g. "112211-bima@gmail.com")
     * @param  {object} data         Data object containing contact information
     * @param  {string} data.externalId   External/loyalty ID (e.g. "112211")
     * @param  {string} data.mobileNumber Mobile number in international format (e.g. "621345627890")
     * @param  {string} data.email        Email address (e.g. "bima@gmail.com")
     * @param  {string} data.locale       Locale code (e.g. "id", "en")
     *   @param  {string}  [data.firstName]  Optional. First name
     *   @param  {string}  [data.lastName]   Optional. Last name
     * @param  {object} data.emailConsent      User consent information (e.g. "true" or "false")
     * @param  {object} data.whatsappConsent      User consent information (e.g. "true" or "false")
     * @param  {string} accessToken  Valid SFMC Bearer access token
     * @return {object}              { valid: boolean, contactKey?: string, reason?: string, statusCode?: number }
     */
    function createContact(contactKey, data, accessToken) {
        try {
            var endpoint = BASE_URL + "/contacts/v1/contacts";

            var attributeSets = [
                {
                    name: "EraLoyalty_External_ID",
                    items: [
                        {
                            values: [
                                { name: "Contact_Key", value: contactKey },
                                { name: "External_Id", value: data.externalId }
                            ]
                        }
                    ]
                }
            ];

            // If has email, passing 'Email Opt Out' and 'Email Demographics'
            if (data.email) {
                var emailDemographics = [
                    {
                        name: "Email Opt Out",
                        value: data.emailConsent || "false"
                    }
                ];
                // ── Email Demographics: build "Name" from whatever name parts exist ────
                var nameParts = [];
                if (data.firstName && String(data.firstName).length > 0)
                    nameParts.push(data.firstName);
                if (data.lastName && String(data.lastName).length > 0)
                    nameParts.push(data.lastName);
                var displayName = nameParts.length > 0 ? nameParts.join(" ") : "";
                if (displayName) {
                    emailDemographics.push({ name: "Name", value: displayName });
                }

                attributeSets.push({
                    name: "Email Addresses",
                    items: [
                        {
                            values: [
                                { name: "Email Address", value: data.email },
                                { name: "HTML Enabled", value: "true" }
                            ]
                        }
                    ]
                });

                attributeSets.push({
                    name: "Email Demographics",
                    items: [
                        {
                            values: emailDemographics
                        }
                    ]
                });
            }

            // If has mobile number, passing 'MobileConnect Demographics' attribute
            if (data.mobileNumber) {
                // ── MobileConnect Demographics values (required fields first) ──────────
                var mobileValues = [
                    {
                        name: "Mobile Number",
                        value: data.mobileNumber
                    },
                    { name: "Locale", value: data.locale },
                    { name: "Status", value: data.whatsappConsent || "0" },
                    { name: "Channel", value: "Mobile" }
                ];

                // Only include First Name / Last Name when provided and non-empty
                if (data.firstName && String(data.firstName).length > 0) {
                    mobileValues.push({
                        name: "First Name",
                        value: data.firstName
                    });
                }
                if (data.lastName && String(data.lastName).length > 0) {
                    mobileValues.push({ name: "Last Name", value: data.lastName });
                }

                attributeSets.push({
                    name: "MobileConnect Demographics",
                    items: [
                        {
                            values: mobileValues
                        }
                    ]
                });
            }

            var payload = {
                contactKey: contactKey,
                attributeSets: attributeSets
            };

            var result = HTTP.Post(
                endpoint,
                "application/json",
                Stringify(payload),
                ["Authorization"],
                ["Bearer " + accessToken]
            );

            if (result.StatusCode === 200 || result.StatusCode === 201) {
                var parsedResponse = Platform.Function.ParseJSON(
                    result.Response[0]
                );

                return {
                    valid: true,
                    contactKey: parsedResponse.contactKey || contactKey
                };
            } else {
                var errBody =
                    result.Response && result.Response[0]
                        ? String(result.Response[0])
                        : "";

                return {
                    valid: false,
                    reason: "Create Contact Error - Response: " + errBody,
                    statusCode: result.StatusCode
                };
            }
        } catch (ex) {
            return {
                valid: false,
                reason: ex.toString()
            };
        }
    }

    /**
     * Detects the country from a phone number or dial code.
     *
     * Matching strategy (longest-prefix-first):
     *   1. Try up to 8-digit prefix (for NANP +1XXX and special codes like +441481).
     *   2. Walk down to 1-digit prefix.
     *   3. Returns null if no match is found.
     *
     * @param  {string|number} phone  Full phone number (e.g. "6281234567890") OR
     *                                bare dial code (e.g. "62", 62, "+62").
     * @return {object|null}  { dialCode, country, iso2, iso3 }  or null if not found.
     *
     * Usage examples:
     *   detectCountryFromPhone("6281234567890")  → { dialCode: "62", country: "Indonesia", iso2: "ID", iso3: "IDN" }
     *   detectCountryFromPhone("+6281234567890") → same
     *   detectCountryFromPhone("62")             → same
     *   detectCountryFromPhone("12125551234")    → { dialCode: "1212", country: "United States", iso2: "US", iso3: "USA" }
     */
    function detectCountryFromPhone(phone) {
        // ── 1. Normalise input ──────────────────────────────────────────────────
        var digits = String(phone).replace(/^\+/, "").replace(/\D/g, "");
        if (!digits || digits.length === 0) return null;

        // ── 2. Dial-code lookup table (longest-match-first for ties) ────────────
        //
        //  Format: dialCode → [country, iso2, iso3]
        //
        //  Notes:
        //   • NANP (+1) area codes are listed individually so "+1212" → New York, USA
        //     rather than generic "+1" → United States.
        //   • A few codes share a dial code (e.g. +262 Mayotte/Réunion); first entry wins.
        //   • Russian zones +76/+77 both map to Kazakhstan by convention; +7 maps to Russia.
        //

        var DB = {
            // ── 6-digit special codes ──────────────────────────────────────
            441481: ["Guernsey", "GG", "GGY"],
            441534: ["Jersey", "JE", "JEY"],
            441624: ["Isle of Man", "IM", "IMN"],

            // ── NANP: Caribbean & territories (+1 + 3-digit area code) ─────
            1242: ["Bahamas", "BS", "BHS"],
            1246: ["Barbados", "BB", "BRB"],
            1264: ["Anguilla", "AI", "AIA"],
            1268: ["Antigua and Barbuda", "AG", "ATG"],
            1284: ["British Virgin Islands", "VG", "VGB"],
            1340: ["United States Virgin Islands", "VI", "VIR"],
            1345: ["Cayman Islands", "KY", "CYM"],
            1441: ["Bermuda", "BM", "BMU"],
            1473: ["Grenada", "GD", "GRD"],
            1649: ["Turks and Caicos Islands", "TC", "TCA"],
            1664: ["Montserrat", "MS", "MSR"],
            1670: ["Northern Mariana Islands", "MP", "MNP"],
            1671: ["Guam", "GU", "GUM"],
            1684: ["American Samoa", "AS", "ASM"],
            1721: ["Sint Maarten", "SX", "SXM"],
            1758: ["Saint Lucia", "LC", "LCA"],
            1767: ["Dominica", "DM", "DMA"],
            1784: ["Saint Vincent and the Grenadines", "VC", "VCT"],
            1787: ["Puerto Rico", "PR", "PRI"],
            1809: ["Dominican Republic", "DO", "DOM"],
            1829: ["Dominican Republic", "DO", "DOM"],
            1849: ["Dominican Republic", "DO", "DOM"],
            1868: ["Trinidad and Tobago", "TT", "TTO"],
            1869: ["Saint Kitts and Nevis", "KN", "KNA"],
            1876: ["Jamaica", "JM", "JAM"],
            1939: ["Puerto Rico", "PR", "PRI"],

            // ── NANP: United States area codes ────────────────────────────
            1201: ["United States", "US", "USA"],
            1202: ["United States", "US", "USA"],
            1203: ["United States", "US", "USA"],
            1205: ["United States", "US", "USA"],
            1206: ["United States", "US", "USA"],
            1207: ["United States", "US", "USA"],
            1208: ["United States", "US", "USA"],
            1209: ["United States", "US", "USA"],
            1210: ["United States", "US", "USA"],
            1212: ["United States", "US", "USA"],
            1213: ["United States", "US", "USA"],
            1214: ["United States", "US", "USA"],
            1215: ["United States", "US", "USA"],
            1216: ["United States", "US", "USA"],
            1217: ["United States", "US", "USA"],
            1218: ["United States", "US", "USA"],
            1219: ["United States", "US", "USA"],
            1224: ["United States", "US", "USA"],
            1225: ["United States", "US", "USA"],
            1228: ["United States", "US", "USA"],
            1229: ["United States", "US", "USA"],
            1231: ["United States", "US", "USA"],
            1234: ["United States", "US", "USA"],
            1239: ["United States", "US", "USA"],
            1240: ["United States", "US", "USA"],
            1248: ["United States", "US", "USA"],
            1251: ["United States", "US", "USA"],
            1252: ["United States", "US", "USA"],
            1253: ["United States", "US", "USA"],
            1254: ["United States", "US", "USA"],
            1256: ["United States", "US", "USA"],
            1260: ["United States", "US", "USA"],
            1262: ["United States", "US", "USA"],
            1267: ["United States", "US", "USA"],
            1269: ["United States", "US", "USA"],
            1270: ["United States", "US", "USA"],
            1276: ["United States", "US", "USA"],
            1278: ["United States", "US", "USA"],
            1280: ["United States", "US", "USA"],
            1281: ["United States", "US", "USA"],
            1282: ["United States", "US", "USA"],
            1283: ["United States", "US", "USA"],
            1301: ["United States", "US", "USA"],
            1302: ["United States", "US", "USA"],
            1303: ["United States", "US", "USA"],
            1304: ["United States", "US", "USA"],
            1305: ["United States", "US", "USA"],
            1307: ["United States", "US", "USA"],
            1308: ["United States", "US", "USA"],
            1309: ["United States", "US", "USA"],
            1310: ["United States", "US", "USA"],
            1312: ["United States", "US", "USA"],
            1313: ["United States", "US", "USA"],
            1314: ["United States", "US", "USA"],
            1315: ["United States", "US", "USA"],
            1316: ["United States", "US", "USA"],
            1317: ["United States", "US", "USA"],
            1318: ["United States", "US", "USA"],
            1319: ["United States", "US", "USA"],
            1320: ["United States", "US", "USA"],
            1321: ["United States", "US", "USA"],
            1323: ["United States", "US", "USA"],
            1325: ["United States", "US", "USA"],
            1327: ["United States", "US", "USA"],
            1330: ["United States", "US", "USA"],
            1331: ["United States", "US", "USA"],
            1334: ["United States", "US", "USA"],
            1336: ["United States", "US", "USA"],
            1337: ["United States", "US", "USA"],
            1339: ["United States", "US", "USA"],
            1341: ["United States", "US", "USA"],
            1347: ["United States", "US", "USA"],
            1351: ["United States", "US", "USA"],
            1352: ["United States", "US", "USA"],
            1358: ["United States", "US", "USA"],
            1360: ["United States", "US", "USA"],
            1361: ["United States", "US", "USA"],
            1369: ["United States", "US", "USA"],
            1380: ["United States", "US", "USA"],
            1381: ["United States", "US", "USA"],
            1383: ["United States", "US", "USA"],
            1385: ["United States", "US", "USA"],
            1386: ["United States", "US", "USA"],
            1401: ["United States", "US", "USA"],
            1402: ["United States", "US", "USA"],
            1404: ["United States", "US", "USA"],
            1405: ["United States", "US", "USA"],
            1406: ["United States", "US", "USA"],
            1407: ["United States", "US", "USA"],
            1408: ["United States", "US", "USA"],
            1409: ["United States", "US", "USA"],
            1410: ["United States", "US", "USA"],
            1412: ["United States", "US", "USA"],
            1413: ["United States", "US", "USA"],
            1414: ["United States", "US", "USA"],
            1415: ["United States", "US", "USA"],
            1417: ["United States", "US", "USA"],
            1419: ["United States", "US", "USA"],
            1420: ["United States", "US", "USA"],
            1423: ["United States", "US", "USA"],
            1424: ["United States", "US", "USA"],
            1425: ["United States", "US", "USA"],
            1430: ["United States", "US", "USA"],
            1432: ["United States", "US", "USA"],
            1434: ["United States", "US", "USA"],
            1435: ["United States", "US", "USA"],
            1440: ["United States", "US", "USA"],
            1442: ["United States", "US", "USA"],
            1443: ["United States", "US", "USA"],
            1445: ["United States", "US", "USA"],
            1464: ["United States", "US", "USA"],
            1469: ["United States", "US", "USA"],
            1470: ["United States", "US", "USA"],
            1475: ["United States", "US", "USA"],
            1478: ["United States", "US", "USA"],
            1479: ["United States", "US", "USA"],
            1480: ["United States", "US", "USA"],
            1484: ["United States", "US", "USA"],
            1501: ["United States", "US", "USA"],
            1502: ["United States", "US", "USA"],
            1503: ["United States", "US", "USA"],
            1504: ["United States", "US", "USA"],
            1505: ["United States", "US", "USA"],
            1507: ["United States", "US", "USA"],
            1508: ["United States", "US", "USA"],
            1509: ["United States", "US", "USA"],
            1510: ["United States", "US", "USA"],
            1512: ["United States", "US", "USA"],
            1513: ["United States", "US", "USA"],
            1515: ["United States", "US", "USA"],
            1516: ["United States", "US", "USA"],
            1517: ["United States", "US", "USA"],
            1518: ["United States", "US", "USA"],
            1520: ["United States", "US", "USA"],
            1530: ["United States", "US", "USA"],
            1540: ["United States", "US", "USA"],
            1541: ["United States", "US", "USA"],
            1546: ["United States", "US", "USA"],
            1551: ["United States", "US", "USA"],
            1557: ["United States", "US", "USA"],
            1559: ["United States", "US", "USA"],
            1561: ["United States", "US", "USA"],
            1562: ["United States", "US", "USA"],
            1563: ["United States", "US", "USA"],
            1564: ["United States", "US", "USA"],
            1567: ["United States", "US", "USA"],
            1570: ["United States", "US", "USA"],
            1571: ["United States", "US", "USA"],
            1573: ["United States", "US", "USA"],
            1574: ["United States", "US", "USA"],
            1580: ["United States", "US", "USA"],
            1585: ["United States", "US", "USA"],
            1586: ["United States", "US", "USA"],
            1601: ["United States", "US", "USA"],
            1602: ["United States", "US", "USA"],
            1603: ["United States", "US", "USA"],
            1605: ["United States", "US", "USA"],
            1606: ["United States", "US", "USA"],
            1607: ["United States", "US", "USA"],
            1608: ["United States", "US", "USA"],
            1609: ["United States", "US", "USA"],
            1610: ["United States", "US", "USA"],
            1612: ["United States", "US", "USA"],
            1614: ["United States", "US", "USA"],
            1615: ["United States", "US", "USA"],
            1616: ["United States", "US", "USA"],
            1617: ["United States", "US", "USA"],
            1618: ["United States", "US", "USA"],
            1619: ["United States", "US", "USA"],
            1620: ["United States", "US", "USA"],
            1623: ["United States", "US", "USA"],
            1626: ["United States", "US", "USA"],
            1627: ["United States", "US", "USA"],
            1628: ["United States", "US", "USA"],
            1630: ["United States", "US", "USA"],
            1631: ["United States", "US", "USA"],
            1636: ["United States", "US", "USA"],
            1641: ["United States", "US", "USA"],
            1646: ["United States", "US", "USA"],
            1650: ["United States", "US", "USA"],
            1651: ["United States", "US", "USA"],
            1657: ["United States", "US", "USA"],
            1660: ["United States", "US", "USA"],
            1661: ["United States", "US", "USA"],
            1662: ["United States", "US", "USA"],
            1669: ["United States", "US", "USA"],
            1678: ["United States", "US", "USA"],
            1679: ["United States", "US", "USA"],
            1682: ["United States", "US", "USA"],
            1689: ["United States", "US", "USA"],
            1701: ["United States", "US", "USA"],
            1702: ["United States", "US", "USA"],
            1703: ["United States", "US", "USA"],
            1704: ["United States", "US", "USA"],
            1706: ["United States", "US", "USA"],
            1707: ["United States", "US", "USA"],
            1708: ["United States", "US", "USA"],
            1710: ["United States", "US", "USA"],
            1712: ["United States", "US", "USA"],
            1713: ["United States", "US", "USA"],
            1714: ["United States", "US", "USA"],
            1715: ["United States", "US", "USA"],
            1716: ["United States", "US", "USA"],
            1717: ["United States", "US", "USA"],
            1718: ["United States", "US", "USA"],
            1719: ["United States", "US", "USA"],
            1720: ["United States", "US", "USA"],
            1724: ["United States", "US", "USA"],
            1727: ["United States", "US", "USA"],
            1731: ["United States", "US", "USA"],
            1732: ["United States", "US", "USA"],
            1734: ["United States", "US", "USA"],
            1737: ["United States", "US", "USA"],
            1740: ["United States", "US", "USA"],
            1747: ["United States", "US", "USA"],
            1752: ["United States", "US", "USA"],
            1754: ["United States", "US", "USA"],
            1757: ["United States", "US", "USA"],
            1760: ["United States", "US", "USA"],
            1763: ["United States", "US", "USA"],
            1764: ["United States", "US", "USA"],
            1765: ["United States", "US", "USA"],
            1770: ["United States", "US", "USA"],
            1772: ["United States", "US", "USA"],
            1773: ["United States", "US", "USA"],
            1774: ["United States", "US", "USA"],
            1775: ["United States", "US", "USA"],
            1781: ["United States", "US", "USA"],
            1785: ["United States", "US", "USA"],
            1786: ["United States", "US", "USA"],
            1801: ["United States", "US", "USA"],
            1802: ["United States", "US", "USA"],
            1803: ["United States", "US", "USA"],
            1804: ["United States", "US", "USA"],
            1805: ["United States", "US", "USA"],
            1806: ["United States", "US", "USA"],
            1808: ["United States", "US", "USA"],
            1810: ["United States", "US", "USA"],
            1812: ["United States", "US", "USA"],
            1813: ["United States", "US", "USA"],
            1814: ["United States", "US", "USA"],
            1815: ["United States", "US", "USA"],
            1816: ["United States", "US", "USA"],
            1817: ["United States", "US", "USA"],
            1818: ["United States", "US", "USA"],
            1828: ["United States", "US", "USA"],
            1830: ["United States", "US", "USA"],
            1831: ["United States", "US", "USA"],
            1832: ["United States", "US", "USA"],
            1835: ["United States", "US", "USA"],
            1836: ["United States", "US", "USA"],
            1843: ["United States", "US", "USA"],
            1845: ["United States", "US", "USA"],
            1847: ["United States", "US", "USA"],
            1848: ["United States", "US", "USA"],
            1850: ["United States", "US", "USA"],
            1856: ["United States", "US", "USA"],
            1857: ["United States", "US", "USA"],
            1858: ["United States", "US", "USA"],
            1859: ["United States", "US", "USA"],
            1860: ["United States", "US", "USA"],
            1861: ["United States", "US", "USA"],
            1862: ["United States", "US", "USA"],
            1863: ["United States", "US", "USA"],
            1864: ["United States", "US", "USA"],
            1865: ["United States", "US", "USA"],
            1870: ["United States", "US", "USA"],
            1872: ["United States", "US", "USA"],
            1878: ["United States", "US", "USA"],
            1900: ["United States", "US", "USA"],
            1901: ["United States", "US", "USA"],
            1903: ["United States", "US", "USA"],
            1904: ["United States", "US", "USA"],
            1906: ["United States", "US", "USA"],
            1907: ["United States", "US", "USA"],
            1908: ["United States", "US", "USA"],
            1909: ["United States", "US", "USA"],
            1910: ["United States", "US", "USA"],
            1912: ["United States", "US", "USA"],
            1913: ["United States", "US", "USA"],
            1914: ["United States", "US", "USA"],
            1915: ["United States", "US", "USA"],
            1916: ["United States", "US", "USA"],
            1917: ["United States", "US", "USA"],
            1918: ["United States", "US", "USA"],
            1919: ["United States", "US", "USA"],
            1920: ["United States", "US", "USA"],
            1925: ["United States", "US", "USA"],
            1928: ["United States", "US", "USA"],
            1931: ["United States", "US", "USA"],
            1935: ["United States", "US", "USA"],
            1936: ["United States", "US", "USA"],
            1937: ["United States", "US", "USA"],
            1940: ["United States", "US", "USA"],
            1941: ["United States", "US", "USA"],
            1947: ["United States", "US", "USA"],
            1949: ["United States", "US", "USA"],
            1951: ["United States", "US", "USA"],
            1952: ["United States", "US", "USA"],
            1954: ["United States", "US", "USA"],
            1956: ["United States", "US", "USA"],
            1957: ["United States", "US", "USA"],
            1959: ["United States", "US", "USA"],
            1969: ["United States", "US", "USA"],
            1970: ["United States", "US", "USA"],
            1971: ["United States", "US", "USA"],
            1972: ["United States", "US", "USA"],
            1973: ["United States", "US", "USA"],
            1975: ["United States", "US", "USA"],
            1978: ["United States", "US", "USA"],
            1979: ["United States", "US", "USA"],
            1980: ["United States", "US", "USA"],
            1984: ["United States", "US", "USA"],
            1985: ["United States", "US", "USA"],
            1989: ["United States", "US", "USA"],

            // ── NANP: Canada area codes ────────────────────────────────────
            1204: ["Canada", "CA", "CAN"],
            1226: ["Canada", "CA", "CAN"],
            1236: ["Canada", "CA", "CAN"],
            1249: ["Canada", "CA", "CAN"],
            1250: ["Canada", "CA", "CAN"],
            1289: ["Canada", "CA", "CAN"],
            1306: ["Canada", "CA", "CAN"],
            1343: ["Canada", "CA", "CAN"],
            1354: ["Canada", "CA", "CAN"],
            1365: ["Canada", "CA", "CAN"],
            1367: ["Canada", "CA", "CAN"],
            1403: ["Canada", "CA", "CAN"],
            1416: ["Canada", "CA", "CAN"],
            1418: ["Canada", "CA", "CAN"],
            1431: ["Canada", "CA", "CAN"],
            1437: ["Canada", "CA", "CAN"],
            1438: ["Canada", "CA", "CAN"],
            1450: ["Canada", "CA", "CAN"],
            1506: ["Canada", "CA", "CAN"],
            1514: ["Canada", "CA", "CAN"],
            1519: ["Canada", "CA", "CAN"],
            1548: ["Canada", "CA", "CAN"],
            1579: ["Canada", "CA", "CAN"],
            1581: ["Canada", "CA", "CAN"],
            1587: ["Canada", "CA", "CAN"],
            1600: ["Canada", "CA", "CAN"],
            1604: ["Canada", "CA", "CAN"],
            1613: ["Canada", "CA", "CAN"],
            1639: ["Canada", "CA", "CAN"],
            1647: ["Canada", "CA", "CAN"],
            1705: ["Canada", "CA", "CAN"],
            1709: ["Canada", "CA", "CAN"],
            1778: ["Canada", "CA", "CAN"],
            1780: ["Canada", "CA", "CAN"],
            1782: ["Canada", "CA", "CAN"],
            1807: ["Canada", "CA", "CAN"],
            1819: ["Canada", "CA", "CAN"],
            1825: ["Canada", "CA", "CAN"],
            1867: ["Canada", "CA", "CAN"],
            1873: ["Canada", "CA", "CAN"],
            1902: ["Canada", "CA", "CAN"],
            1905: ["Canada", "CA", "CAN"],

            // ── 3-digit codes ──────────────────────────────────────────────
            211: ["South Sudan", "SS", "SSD"],
            212: ["Morocco", "MA", "MAR"],
            213: ["Algeria", "DZ", "DZA"],
            216: ["Tunisia", "TN", "TUN"],
            218: ["Libya", "LY", "LBY"],
            220: ["Gambia", "GM", "GMB"],
            221: ["Senegal", "SN", "SEN"],
            222: ["Mauritania", "MR", "MRT"],
            223: ["Mali", "ML", "MLI"],
            224: ["Guinea", "GN", "GIN"],
            225: ["Ivory Coast", "CI", "CIV"],
            226: ["Burkina Faso", "BF", "BFA"],
            227: ["Niger", "NE", "NER"],
            228: ["Togo", "TG", "TGO"],
            229: ["Benin", "BJ", "BEN"],
            230: ["Mauritius", "MU", "MUS"],
            231: ["Liberia", "LR", "LBR"],
            232: ["Sierra Leone", "SL", "SLE"],
            233: ["Ghana", "GH", "GHA"],
            234: ["Nigeria", "NG", "NGA"],
            235: ["Chad", "TD", "TCD"],
            236: ["Central African Republic", "CF", "CAF"],
            237: ["Cameroon", "CM", "CMR"],
            238: ["Cape Verde", "CV", "CPV"],
            239: ["Sao Tome and Principe", "ST", "STP"],
            240: ["Equatorial Guinea", "GQ", "GNQ"],
            241: ["Gabon", "GA", "GAB"],
            242: ["Republic of the Congo", "CG", "COG"],
            243: ["Democratic Republic of the Congo", "CD", "COD"],
            244: ["Angola", "AO", "AGO"],
            245: ["Guinea-Bissau", "GW", "GNB"],
            246: ["British Indian Ocean Territory", "IO", "IOT"],
            248: ["Seychelles", "SC", "SYC"],
            249: ["Sudan", "SD", "SDN"],
            250: ["Rwanda", "RW", "RWA"],
            251: ["Ethiopia", "ET", "ETH"],
            252: ["Somalia", "SO", "SOM"],
            253: ["Djibouti", "DJ", "DJI"],
            254: ["Kenya", "KE", "KEN"],
            255: ["Tanzania", "TZ", "TZA"],
            256: ["Uganda", "UG", "UGA"],
            257: ["Burundi", "BI", "BDI"],
            258: ["Mozambique", "MZ", "MOZ"],
            260: ["Zambia", "ZM", "ZMB"],
            261: ["Madagascar", "MG", "MDG"],
            262: ["Mayotte / Reunion", "", ""],
            263: ["Zimbabwe", "ZW", "ZWE"],
            264: ["Namibia", "NA", "NAM"],
            265: ["Malawi", "MW", "MWI"],
            266: ["Lesotho", "LS", "LSO"],
            267: ["Botswana", "BW", "BWA"],
            268: ["Eswatini", "SZ", "SWZ"],
            269: ["Comoros", "KM", "COM"],
            290: ["Saint Helena", "SH", "SHN"],
            291: ["Eritrea", "ER", "ERI"],
            297: ["Aruba", "AW", "ABW"],
            298: ["Faroe Islands", "FO", "FRO"],
            299: ["Greenland", "GL", "GRL"],
            350: ["Gibraltar", "GI", "GIB"],
            351: ["Portugal", "PT", "PRT"],
            352: ["Luxembourg", "LU", "LUX"],
            353: ["Ireland", "IE", "IRL"],
            354: ["Iceland", "IS", "ISL"],
            355: ["Albania", "AL", "ALB"],
            356: ["Malta", "MT", "MLT"],
            357: ["Cyprus", "CY", "CYP"],
            358: ["Finland", "FI", "FIN"],
            359: ["Bulgaria", "BG", "BGR"],
            370: ["Lithuania", "LT", "LTU"],
            371: ["Latvia", "LV", "LVA"],
            372: ["Estonia", "EE", "EST"],
            373: ["Moldova", "MD", "MDA"],
            374: ["Armenia", "AM", "ARM"],
            375: ["Belarus", "BY", "BLR"],
            376: ["Andorra", "AD", "AND"],
            377: ["Monaco", "MC", "MCO"],
            378: ["San Marino", "SM", "SMR"],
            379: ["Vatican", "VA", "VAT"],
            380: ["Ukraine", "UA", "UKR"],
            381: ["Serbia", "RS", "SRB"],
            382: ["Montenegro", "ME", "MNE"],
            383: ["Kosovo", "XK", "XKX"],
            385: ["Croatia", "HR", "HRV"],
            386: ["Slovenia", "SI", "SVN"],
            387: ["Bosnia and Herzegovina", "BA", "BIH"],
            389: ["North Macedonia", "MK", "MKD"],
            420: ["Czech Republic", "CZ", "CZE"],
            421: ["Slovakia", "SK", "SVK"],
            423: ["Liechtenstein", "LI", "LIE"],
            500: ["Falkland Islands", "FK", "FLK"],
            501: ["Belize", "BZ", "BLZ"],
            502: ["Guatemala", "GT", "GTM"],
            503: ["El Salvador", "SV", "SLV"],
            504: ["Honduras", "HN", "HND"],
            505: ["Nicaragua", "NI", "NIC"],
            506: ["Costa Rica", "CR", "CRI"],
            507: ["Panama", "PA", "PAN"],
            508: ["Saint Pierre and Miquelon", "PM", "SPM"],
            509: ["Haiti", "HT", "HTI"],
            590: ["Saint Barthelemy / Saint Martin", "", ""],
            591: ["Bolivia", "BO", "BOL"],
            592: ["Guyana", "GY", "GUY"],
            593: ["Ecuador", "EC", "ECU"],
            595: ["Paraguay", "PY", "PRY"],
            597: ["Suriname", "SR", "SUR"],
            598: ["Uruguay", "UY", "URY"],
            599: ["Curacao", "CW", "CUW"],
            670: ["East Timor", "TL", "TLS"],
            672: ["Antarctica", "AQ", "ATA"],
            673: ["Brunei", "BN", "BRN"],
            674: ["Nauru", "NR", "NRU"],
            675: ["Papua New Guinea", "PG", "PNG"],
            676: ["Tonga", "TO", "TON"],
            677: ["Solomon Islands", "SB", "SLB"],
            678: ["Vanuatu", "VU", "VUT"],
            679: ["Fiji", "FJ", "FJI"],
            680: ["Palau", "PW", "PLW"],
            681: ["Wallis and Futuna", "WF", "WLF"],
            682: ["Cook Islands", "CK", "COK"],
            683: ["Niue", "NU", "NIU"],
            685: ["Samoa", "WS", "WSM"],
            686: ["Kiribati", "KI", "KIR"],
            687: ["New Caledonia", "NC", "NCL"],
            688: ["Tuvalu", "TV", "TUV"],
            689: ["French Polynesia", "PF", "PYF"],
            690: ["Tokelau", "TK", "TKL"],
            691: ["Micronesia", "FM", "FSM"],
            692: ["Marshall Islands", "MH", "MHL"],
            850: ["North Korea", "KP", "PRK"],
            852: ["Hong Kong", "HK", "HKG"],
            853: ["Macau", "MO", "MAC"],
            855: ["Cambodia", "KH", "KHM"],
            856: ["Laos", "LA", "LAO"],
            880: ["Bangladesh", "BD", "BGD"],
            886: ["Taiwan", "TW", "TWN"],
            960: ["Maldives", "MV", "MDV"],
            961: ["Lebanon", "LB", "LBN"],
            962: ["Jordan", "JO", "JOR"],
            963: ["Syria", "SY", "SYR"],
            964: ["Iraq", "IQ", "IRQ"],
            965: ["Kuwait", "KW", "KWT"],
            966: ["Saudi Arabia", "SA", "SAU"],
            967: ["Yemen", "YE", "YEM"],
            968: ["Oman", "OM", "OMN"],
            970: ["Palestine", "PS", "PSE"],
            971: ["United Arab Emirates", "AE", "ARE"],
            972: ["Israel", "IL", "ISR"],
            973: ["Bahrain", "BH", "BHR"],
            974: ["Qatar", "QA", "QAT"],
            975: ["Bhutan", "BT", "BTN"],
            976: ["Mongolia", "MN", "MNG"],
            977: ["Nepal", "NP", "NPL"],
            992: ["Tajikistan", "TJ", "TJK"],
            993: ["Turkmenistan", "TM", "TKM"],
            994: ["Azerbaijan", "AZ", "AZE"],
            995: ["Georgia", "GE", "GEO"],
            996: ["Kyrgyzstan", "KG", "KGZ"],
            998: ["Uzbekistan", "UZ", "UZB"],

            // ── 2-digit codes ──────────────────────────────────────────────
            20: ["Egypt", "EG", "EGY"],
            27: ["South Africa", "ZA", "ZAF"],
            30: ["Greece", "GR", "GRC"],
            31: ["Netherlands", "NL", "NLD"],
            32: ["Belgium", "BE", "BEL"],
            33: ["France", "FR", "FRA"],
            34: ["Spain", "ES", "ESP"],
            36: ["Hungary", "HU", "HUN"],
            39: ["Italy", "IT", "ITA"],
            40: ["Romania", "RO", "ROU"],
            41: ["Switzerland", "CH", "CHE"],
            43: ["Austria", "AT", "AUT"],
            44: ["United Kingdom", "GB", "GBR"],
            45: ["Denmark", "DK", "DNK"],
            46: ["Sweden", "SE", "SWE"],
            47: ["Norway", "NO", "NOR"],
            48: ["Poland", "PL", "POL"],
            49: ["Germany", "DE", "DEU"],
            51: ["Peru", "PE", "PER"],
            52: ["Mexico", "MX", "MEX"],
            53: ["Cuba", "CU", "CUB"],
            54: ["Argentina", "AR", "ARG"],
            55: ["Brazil", "BR", "BRA"],
            56: ["Chile", "CL", "CHL"],
            57: ["Colombia", "CO", "COL"],
            58: ["Venezuela", "VE", "VEN"],
            60: ["Malaysia", "MY", "MYS"],
            61: ["Australia", "AU", "AUS"],
            62: ["Indonesia", "ID", "IDN"],
            63: ["Philippines", "PH", "PHL"],
            64: ["New Zealand", "NZ", "NZL"],
            65: ["Singapore", "SG", "SGP"],
            66: ["Thailand", "TH", "THA"],
            76: ["Kazakhstan", "KZ", "KAZ"],
            77: ["Kazakhstan", "KZ", "KAZ"],
            81: ["Japan", "JP", "JPN"],
            82: ["South Korea", "KR", "KOR"],
            84: ["Vietnam", "VN", "VNM"],
            86: ["China", "CN", "CHN"],
            90: ["Turkey", "TR", "TUR"],
            91: ["India", "IN", "IND"],
            92: ["Pakistan", "PK", "PAK"],
            93: ["Afghanistan", "AF", "AFG"],
            94: ["Sri Lanka", "LK", "LKA"],
            95: ["Myanmar", "MM", "MMR"],
            98: ["Iran", "IR", "IRN"],

            // ── 1-digit codes ──────────────────────────────────────────────
            1: ["United States / Canada / NANP", "US", "USA"],
            7: ["Russia", "RU", "RUS"]
        };

        // ── 3. Longest-prefix match (8 → 1 digits) ──────────────────────────────
        var maxLen = digits.length < 8 ? digits.length : 8;
        for (var len = maxLen; len >= 1; len--) {
            var prefix = digits.substring(0, len);
            if (DB.hasOwnProperty(prefix)) {
                var row = DB[prefix];
                return {
                    dialCode: prefix,
                    country: row[0],
                    iso2: row[1],
                    iso3: row[2]
                };
            }
        }

        return null; // no match found
    }

    /**
     * Run fn() up to RETRY_MAX_ATTEMPTS times. Retries only when isRetryable(result) is true (transient failures).
     * fn() must return { valid: boolean, ... }; optional statusCode on failure for retry decision.
     * Returns { result: lastResult, attempts: number } so callers can see if retries were used.
     */
    function withRetry(fn, isRetryable) {
        var lastResult;
        for (var attempt = 1; attempt <= RETRY_MAX_ATTEMPTS; attempt++) {
            lastResult = fn();
            if (lastResult.valid) return { result: lastResult, attempts: attempt };
            if (!isRetryable(lastResult))
                return { result: lastResult, attempts: attempt };
            if (attempt < RETRY_MAX_ATTEMPTS) {
                var until = new Date().getTime() + RETRY_BACKOFF_MS;
                while (new Date().getTime() < until) {}
            }
        }
        return { result: lastResult, attempts: attempt };
    }

    /** Retry auth only on 429, 5xx, or exception (no statusCode). Do not retry 400/401. */
    function isRetryableAuth(r) {
        if (r.valid) return false;
        if (r.statusCode == null) return true;
        return (
            r.statusCode === 429 ||
            r.statusCode === 500 ||
            r.statusCode === 502 ||
            r.statusCode === 503
        );
    }

    /** Retry API calls only on 429, 5xx, or exception. Do not retry 400/404. */
    function isRetryableApi(r) {
        if (r.valid) return false;
        if (r.statusCode == null) return true;
        return (
            r.statusCode === 429 ||
            r.statusCode === 500 ||
            r.statusCode === 502 ||
            r.statusCode === 503
        );
    }

    /**
     * Map HTTP status code (number) to Status header value for response.
     */
    var STATUS_TEXT = {
        200: "200 OK",
        201: "201 Created",
        400: "400 Bad Request",
        401: "401 Unauthorized",
        404: "404 Not Found",
        500: "500 Internal Server Error"
    };

    /**
     * Get access token from Authorization: Bearer <token> header
     */
    function getBearerToken() {
        var authHeader = Platform.Request.GetRequestHeader("Authorization");
        if (!authHeader || authHeader.indexOf("Bearer ") !== 0) {
            return null;
        }
        var token = authHeader.substring(7);
        return token && String(token).length > 0 ? String(token) : null;
    }

    /**
     * Set common response headers (Status, Content-Type, CORS). Call before writing body.
     */
    function setResponseHeaders(statusCode) {
        var statusText = STATUS_TEXT[statusCode] || "500 Internal Server Error";
        // HTTPHeader.SetValue("Status", statusText);
        HTTPHeader.SetValue("Content-Type", "application/json");
        Platform.Response.SetResponseHeader("Access-Control-Allow-Origin", "*");
        Platform.Response.SetResponseHeader(
            "Access-Control-Allow-Methods",
            "GET, POST, OPTIONS"
        );
        Platform.Response.SetResponseHeader(
            "Access-Control-Allow-Headers",
            "Content-Type, Authorization"
        );
    }

    /**
     * Truncate string to max length for DE column safety.
     */
    function truncateStr(val, maxLen) {
        var s = val != null ? String(val) : "";
        return s.length > maxLen ? s.substring(0, maxLen) : s;
    }

    function getErrorLogTimestamp() {
        try {
            var d =
                typeof Platform.Function.Now === "function"
                    ? Platform.Function.Now()
                    : new Date();
            var pad = function (n) {
                n = Number(n);
                return (n < 10 ? "0" : "") + n;
            };
            var y = d.getFullYear();
            var mo = d.getMonth() + 1;
            var day = d.getDate();
            var h = d.getHours();
            var min = d.getMinutes();
            var sec = d.getSeconds();
            if (isNaN(y) || isNaN(mo) || isNaN(day)) return "";
            return (
                String(y) +
                "-" +
                pad(mo) +
                "-" +
                pad(day) +
                " " +
                pad(h) +
                ":" +
                pad(min) +
                ":" +
                pad(sec) +
                ".000"
            );
        } catch (e) {
            return "";
        }
    }

    /**
     * Log error to Loyalty_Error_Logs DE. Never throws; does not block core process.
     * Call only after response has been sent (e.g. after returnError).
     * Uses DataExtension.Init (Customer Key) + Rows.Add; fallback to InsertData (DE Name).
     */
    function logErrorToDE(
        errorType,
        errorMessage,
        stackTrace,
        emailAddress,
        subscriberKey,
        payloadData
    ) {
        var errorLogId,
            typeStr,
            msgStr,
            stackStr,
            emailStr,
            subKeyStr,
            timestamp,
            dataStr;
        try {
            errorLogId = Platform.Function.GUID();
            typeStr = truncateStr(errorType, 100) || "Unknown";
            msgStr = truncateStr(errorMessage, 4000);
            stackStr = truncateStr(
                stackTrace != null && stackTrace !== "" ? String(stackTrace) : "",
                4000
            );
            emailStr = truncateStr(emailAddress, 254) || "";
            subKeyStr = truncateStr(subscriberKey, 254) || "";
            dataStr = truncateStr(
                payloadData != null ? String(payloadData) : "",
                4000
            );
            timestamp = getErrorLogTimestamp();

            // Prefer DataExtension.Init (uses DE Customer Key "Loyalty_Error_Logs")
            try {
                var de = DataExtension.Init(ERROR_LOG_DE);
                de.Rows.Add({
                    ErrorLogID: errorLogId,
                    ErrorType: typeStr,
                    ErrorMessage: msgStr,
                    StackTrace: stackStr,
                    EmailAddress: emailStr,
                    ErrorTimestamp: timestamp,
                    SubscriberKey: subKeyStr,
                    Data: dataStr
                });
                return;
            } catch (deEx) {
                // Fallback: InsertData uses DE *Name* (not Customer Key). Ensure DE Name = "Loyalty_Error_Logs" if you use this path.
                Platform.Function.InsertData(
                    ERROR_LOG_DE,
                    [
                        "ErrorLogID",
                        "ErrorType",
                        "ErrorMessage",
                        "StackTrace",
                        "EmailAddress",
                        "ErrorTimestamp",
                        "SubscriberKey",
                        "Data"
                    ],
                    [
                        errorLogId,
                        typeStr,
                        msgStr,
                        stackStr,
                        emailStr,
                        timestamp,
                        subKeyStr,
                        dataStr
                    ]
                );
            }
        } catch (logEx) {
            // Expose error in response for debugging (remove in production if needed)
            var errMsg = logEx && logEx.toString ? logEx.toString() : String(logEx);
            Write(
                "\n<!-- Loyalty_Error_Logs insert failed: " +
                    errMsg.replace(/-->/g, "") +
                    " -->"
            );
        }
    }

    /**
     * Return error response then log to Loyalty_Error_Logs (after Write). Use for all API error paths.
     */
    function returnErrorAndLog(
        statusCode,
        message,
        errorType,
        stackTrace,
        emailAddress,
        subscriberKey,
        payloadData
    ) {
        var errorText;
        if (typeof message === "string") {
            errorText = message;
        } else if (message && typeof message === "object" && message.error) {
            errorText =
                typeof message.error === "string"
                    ? message.error
                    : Stringify(message.error);
        } else {
            errorText = message ? Stringify(message) : "Unknown error";
        }
        returnError(statusCode, message);
        logErrorToDE(
            errorType,
            errorText,
            stackTrace != null ? stackTrace : "",
            emailAddress != null ? emailAddress : "",
            subscriberKey != null ? subscriberKey : "",
            payloadData != null ? payloadData : ""
        );
    }

    /**
     * Return success response with HTTP status and JSON body.
     */
    function returnSuccess(statusCode, data) {
        setResponseHeaders(statusCode || 200);
        if (data && typeof data === "object") {
            data.accountID = AccountID;
        } else {
            data = { accountID: AccountID, result: data };
        }

        Write(Stringify(data));
    }

    /**
     * Return error response with HTTP status and JSON body.
     * message: string or object (if object with .error, use that; else stringify).
     */
    function returnError(statusCode, message) {
        setResponseHeaders(statusCode || 500);
        var errorText;
        var payload = { success: false, accountID: AccountID };
        if (typeof message === "string") {
            errorText = message;
        } else if (message && typeof message === "object" && message.error) {
            errorText =
                typeof message.error === "string"
                    ? message.error
                    : Stringify(message.error);
            for (var key in message) {
                if (message.hasOwnProperty(key) && key !== "error") {
                    payload[key] = message[key];
                }
            }
        } else {
            errorText = message ? Stringify(message) : "Unknown error";
        }
        payload.error = errorText;
        Write(Stringify(payload));
    }
</script>