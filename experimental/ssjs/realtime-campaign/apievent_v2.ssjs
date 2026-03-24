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
     *     { "ContactKey": "alfin@gmail.com", "EventDefinitionKey": "...", "Data": { "fullname": "...", "slab_name": "Pilot", "reason_for_change": "..." } }
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

    // Error log Data Extension. DataExtension.Init() uses Customer Key; InsertData() fallback uses DE Name. Set both to "Loyalty_Error_Logs" for simplicity.
    // Columns: ErrorLogID (36), ErrorType (100), ErrorMessage (4000), StackTrace (4000), EmailAddress (254), ErrorTimestamp, SubscriberKey (254).
    var ERROR_LOG_DE = "Loyalty_Error_Logs";

    // Retry config for transient API failures (429, 5xx, network/timeout).
    var RETRY_MAX_ATTEMPTS = 3;
    var RETRY_BACKOFF_MS = 1000;

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
        var authAttempts = accessTokenRes.attempts;

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

        // // Validate required fields (PascalCase to match Interaction API)
        if (!requestData.EventDefinitionKey) {
            var dataEmail =
                requestData.Data && requestData.Data.email
                    ? requestData.Data.email
                    : "";
            returnErrorAndLog(
                400,
                "Missing required field: EventDefinitionKey",
                "Validation",
                "",
                dataEmail,
                dataEmail,
                postData || ""
            );
            return;
        }

        var eventDefinitionKey = requestData.EventDefinitionKey;
        var eventData = requestData.Data || {};

        // Validate event data
        if (!eventData.email) {
            returnErrorAndLog(
                400,
                "Missing required field: email",
                "Validation",
                "",
                "",
                "",
                postData || ""
            );
            return;
        }

        var contactKey;
        var contactAttempts = 0;
        var skipLookup =
            requestData.Options &&
            requestData.Options.hasOwnProperty("GetContactKey") &&
            requestData.Options.GetContactKey === "false";
        if (skipLookup) {
            contactKey = eventData.email;
        } else {
            // Step 1: Find contact key by email (with retry for transient failures)
            var contactKeyRes = withRetry(function () {
                return findContactKeyByEmail(eventData.email, accessToken);
            }, isRetryableApi);
            if (!contactKeyRes.result.valid) {
                returnErrorAndLog(
                    404,
                    {
                        success: false,
                        error: "Contact not found for email: " + eventData.email,
                        attempts: contactKeyRes.attempts
                    },
                    "ContactLookup",
                    "",
                    eventData.email,
                    eventData.email,
                    postData || ""
                );
                return;
            }
            contactKey = contactKeyRes.result.contactKey;
            contactAttempts = contactKeyRes.attempts;
        }

        // Step 2: Trigger API Event (with retry for transient failures)
        var eventResult = withRetry(function () {
            return triggerApiEvent(
                contactKey,
                eventDefinitionKey,
                eventData,
                accessToken
            );
        }, isRetryableApi);
        if (!eventResult.result.valid) {
            returnErrorAndLog(
                500,
                {
                    success: false,
                    error: eventResult.result.reason,
                    attempts: eventResult.attempts
                },
                "ApiEvent",
                "",
                eventData.email,
                contactKey,
                postData || ""
            );
            return;
        }

        var successPayload = {
            success: true,
            message: "Event triggered successfully",
            eventInstanceId: eventResult.result.eventInstanceId
        };
        successPayload.attempts = {
            auth: authAttempts,
            contactLookup: contactAttempts,
            trigger: eventResult.attempts
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
                    reason: "Contact not found for email: " + email,
                    statusCode: 200
                };
            } else {
                var errBody =
                    result.Response && result.Response[0]
                        ? String(result.Response[0])
                        : "";
                return {
                    valid: false,
                    reason: "Find Contact Error - Response: " + errBody,
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

    function triggerApiEvent(
        contactKey,
        eventDefinitionKey,
        eventData,
        accessToken
    ) {
        try {
            var endpoint = BASE_URL + "/interaction/v1/events";

            // Prepare event data: required fields + pass-through all eventData keys
            var data = {
                contact_key: contactKey,
                email: contactKey
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
                var errBody =
                    result.Response && result.Response[0]
                        ? String(result.Response[0])
                        : "";
                var reason = "API Event Error - Response: " + errBody;
                try {
                    var errJson = Platform.Function.ParseJSON(errBody);
                    if (errJson && errJson.message) {
                        reason = errJson.message;
                    }
                } catch (e) {}
                return {
                    valid: false,
                    reason: reason,
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

    ///////// UTILS
    /**
     * Current timestamp for error log. Uses Platform.Function.Now() so the time
     * follows the SFMC account/server timezone (e.g. set to WIB for Jakarta).
     */
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
     * Truncate string to max length for DE column safety.
     */
    function truncateStr(val, maxLen) {
        var s = val != null ? String(val) : "";
        return s.length > maxLen ? s.substring(0, maxLen) : s;
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