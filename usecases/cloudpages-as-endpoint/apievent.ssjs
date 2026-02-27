<script runat="server">
    /**
     * CloudPages API Event Endpoint – Workflow
     *
     * This page acts as the source of truth for triggering Marketing Cloud
     * Interaction (API) events. Callers POST with ContactKey, EventDefinitionKey, and Data;
     * the endpoint resolves the contact and fires the event with the provided data.
     *
     * REQUEST
     *   Method: POST
     *   Auth:   Authorization: Bearer <token> (required)
     *   Body:   JSON (same shape as Interaction API):
     *           - ContactKey (string, required) – contact identifier, typically email
     *           - EventDefinitionKey (string, required)
     *           - Data (object, optional) – event payload (fullname, slab_name, etc.)
     *
     *   Example:
     *     { "ContactKey": "alfin.surya@gmail.com", "EventDefinitionKey": "...", "Data": { "fullname": "...", "slab_name": "Pilot", "reason_for_change": "..." } }
     *
     * WORKFLOW
     *   1. Validate Bearer token and parse POST body (ContactKey, EventDefinitionKey, Data).
     *   2. Find contact key by email via Contacts API using ContactKey.
     *   3. Trigger API event with ContactKey, EventDefinitionKey, Data (contact_key + email + Data keys pass-through).
     *   4. Return success with eventInstanceId or an error response.
     *
     * DATA (pass-through)
     *   Data can have different fields per use case. All primitive values are forwarded.
     *
     *   Example – Tier Upgrade: Data: { fullname, slab_name, reason_for_change }
     *   Example – Welcome Campaign: Data: { fullname, slab_name, loyalty_points }
     *
     *   No code changes needed when new Data fields are added; ensure the
     *   Event Definition in Contact Builder includes the corresponding attributes.
     */
    Platform.Load("core", "1");

    var ENV = "dev";
    var BASE_URL =
        ENV === "prod"
            ? "https://{domain}.rest.marketingcloudapis.com"
            : "https://{domain}.rest.marketingcloudapis.com";

    var AUTH_BASE_URL =
        ENV === "prod"
            ? "https://{domain}.auth.marketingcloudapis.com"
            : "https://{domain}.auth.marketingcloudapis.com";

    var ClientID, ClientSecret;
    if (ENV === "prod") {
        ClientID = "{client_id}";
        ClientSecret = "{client_secret}";
    } else {
        ClientID = "{client_id}";
        ClientSecret = "{client_secret}";
    }

    var DEFAULT_ACCOUNT_ID;
    if (ENV === "prod") {
        DEFAULT_ACCOUNT_ID = 0000; // account ID (production)
    } else {
        DEFAULT_ACCOUNT_ID = 1111; // account ID (sandbox)
    }

    var AccountID = DEFAULT_ACCOUNT_ID;

    // Auth token cache (Data Extension). In SFMC, set DE Customer Key (External Key) to exactly "Loyalty_AuthToken_Cache".
    // Columns: CacheKey (Text, Primary Key), AccessToken (Text 2000), ExpiresAt (Number).
    var AUTH_TOKEN_CACHE_DE = "Loyalty_AuthToken_Cache";
    var AUTH_TOKEN_CACHE_KEY = "default" + "_" + ClientID + "_" + AccountID;
    var AUTH_TOKEN_EXPIRY_BUFFER_SEC = 120;

    ///////// Main API Handler
    try {
        var requestData;
        var headerAccountId = Platform.Request.GetRequestHeader("X-Account-ID");

        if (headerAccountId && !isNaN(parseInt(headerAccountId, 10))) {
            AccountID = parseInt(headerAccountId, 10);
        } else {
            AccountID = DEFAULT_ACCOUNT_ID;
        }

        var postData = Platform.Request.GetPostData(); // Try to get POST data first

        // Get Auth Token
        var accessTokenRes = getAuthToken();
        if (!accessTokenRes.valid) {
            returnError(500, {
                success: false,
                error: accessTokenRes.reason
            });
            return;
        }

        var accessToken = accessTokenRes.token;

        // Ensure payload doesn't empty
        if (postData && postData !== "") {
            // POST request
            try {
                requestData = Platform.Function.ParseJSON(postData);
            } catch (ex) {
                returnError(400, "Invalid JSON in request body");
                return;
            }
        } else {
            returnError(400, "Payload was empty");
            return;
        }

        // // Validate required fields (PascalCase to match Interaction API)
        if (!requestData.ContactKey) {
            returnError(400, "Missing required field: ContactKey");
            return;
        }
        if (!requestData.EventDefinitionKey) {
            returnError(400, "Missing required field: EventDefinitionKey");
            return;
        }

        var contactKeyOrEmail = requestData.ContactKey;
        var eventDefinitionKey = requestData.EventDefinitionKey;
        var eventData = requestData.Data || {};

        var contactKey;
        var skipLookup =
            requestData.Options &&
            requestData.Options.hasOwnProperty("GetContactKey") &&
            requestData.Options.GetContactKey === "false";
        if (skipLookup) {
            contactKey = contactKeyOrEmail;
        } else {
            // Step 1: Find contact key by email (ContactKey is typically the email address)
            var contactKeyRes = findContactKeyByEmail(
                contactKeyOrEmail,
                accessToken
            );
            if (!contactKeyRes.valid) {
                returnError(404, {
                    success: false,
                    error: "Contact not found for ContactKey: " + contactKeyOrEmail
                });
                return;
            }
            contactKey = contactKeyRes.contactKey;
        }

        // Step 2: Trigger API Event
        var eventResult = triggerApiEvent(
            contactKey,
            eventDefinitionKey,
            contactKeyOrEmail,
            eventData,
            accessToken
        );
        if (!eventResult.valid) {
            returnError(500, {
                success: false,
                error: eventResult.reason
            });
            return;
        }

        returnSuccess(200, {
            success: true,
            message: "Event triggered successfully",
            eventInstanceId: eventResult.eventInstanceId
        });
    } catch (ex) {
        returnError(500, "Server error: " + ex.toString());
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
            return { valid: false, reason: "Error: " + reason };
        } catch (ex) {
            return {
                valid: false,
                reason: ex.toString()
            };
        }
    }

    function findContactKeyByEmail(email, accessToken) {
        try {
            var endpoint = BASE_URL + "/contacts/v1/addresses/email/search";

            var payload = {
                ChannelAddressList: [email],
                MaximumCount: 1
            };

            var result = HTTP.Post(
                endpoint,
                "application/json",
                Stringify(payload),
                ["Authorization"],
                ["Bearer " + accessToken]
            );

            if (result.StatusCode === 200) {
                var parsedResponse = Platform.Function.ParseJSON(
                    result.Response[0]
                );

                // Check if contact was found
                if (
                    parsedResponse.channelAddressResponseEntities &&
                    parsedResponse.channelAddressResponseEntities.length > 0
                ) {
                    var contactKeyDetails =
                        parsedResponse.channelAddressResponseEntities[0]
                            .contactKeyDetails;

                    // Check if contactKeyDetails is not empty (not found case)
                    if (
                        contactKeyDetails &&
                        contactKeyDetails.length > 0 &&
                        contactKeyDetails[0].contactKey
                    ) {
                        return {
                            valid: true,
                            contactKey: contactKeyDetails[0].contactKey
                        };
                    }
                }

                return {
                    valid: false,
                    reason: "Contact not found for email: " + email
                };
            } else {
                return {
                    valid: false,
                    reason:
                        "Find Contact Error - Response: " + String(parsedResponse)
                };
            }
        } catch (ex) {
            return {
                valid: false,
                reason: ex.toString()
            };
        }
    }

    function triggerApiEvent(
        contactKey,
        eventDefinitionKey,
        email,
        eventData,
        accessToken
    ) {
        try {
            var endpoint = BASE_URL + "/interaction/v1/events";

            // Prepare event data: required fields + pass-through all eventData keys
            var data = {
                contact_key: contactKey,
                email: email
            };
            for (var key in eventData) {
                if (eventData.hasOwnProperty(key)) {
                    var val = eventData[key];
                    // Only include primitive values safe for the API
                    if (
                        typeof val === "string" ||
                        typeof val === "number" ||
                        typeof val === "boolean"
                    ) {
                        data[key] = val;
                    } else if (val === null) {
                        data[key] = val;
                    }
                }
            }

            var payload = {
                ContactKey: contactKey,
                EventDefinitionKey: eventDefinitionKey,
                Data: data
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
                    eventInstanceId: parsedResponse.eventInstanceId
                };
            } else {
                return {
                    valid: false,
                    reason: "API Event Error - Response: " + String(parsedResponse)
                };
            }
        } catch (ex) {
            return {
                valid: false,
                reason: ex.toString()
            };
        }
    }

    ///////// UTILS
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
        Write(
            Stringify({
                success: false,
                error: errorText,
                accountID: AccountID
            })
        );
    }
</script>