<script runat="server">
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

        // Validate required fields (PascalCase to match Interaction API)
        if (!requestData.action) {
            returnError(
                400,
                "action parameter is required (unsubscribe or subscribe)"
            );
            return;
        }
        if (!requestData.emailAddress) {
            returnError(400, "emailAddress are required");
            return;
        }

        var action = requestData.action;
        var emailAddress = requestData.emailAddress;

        // Route to appropriate function based on action
        var result;
        switch (action.toLowerCase()) {
            case "unsubscribe":
                result = globalUnsubscribe(emailAddress);
                break;

            case "subscribe":
                result = globalSubscribe(emailAddress);
                break;

            default:
                returnError(400, {
                    success: false,
                    error: "Invalid action. Allowed values: 'unsubscribe' or 'subscribe'"
                });
                return;
        }

        // Send response
        if (result.success) {
            returnSuccess(200, result);
            return;
        } else {
            returnError(500, result);
            return;
        }
    } catch (ex) {
        returnError(500, {
            success: false,
            error: "Server error: " + ex.toString()
        });
        return;
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

    // Function for global unsubscribe
    function globalUnsubscribe(emailAddress) {
        try {
            var prox = new Script.Util.WSProxy();

            var props = {
                SubscriberKey: emailAddress,
                EmailAddress: emailAddress,
                Status: "Unsubscribed"
            };

            var options = {
                SaveOptions: [
                    {
                        PropertyName: "*",
                        SaveAction: "UpdateAdd"
                    }
                ]
            };

            var result = prox.updateItem("Subscriber", props, options);

            return {
                success: result.Status === "OK",
                status: result.Status,
                message:
                    result.Status === "OK"
                        ? "Subscriber globally unsubscribed successfully"
                        : "Global unsubscribe failed",
                details: result.Results
            };
        } catch (ex) {
            return {
                success: false,
                status: "Error",
                message: "Exception occurred",
                error: ex.toString()
            };
        }
    }

    // Function for global subscribe
    function globalSubscribe(emailAddress) {
        try {
            var prox = new Script.Util.WSProxy();

            var props = {
                SubscriberKey: emailAddress,
                EmailAddress: emailAddress,
                Status: "Active"
            };

            var options = {
                SaveOptions: [
                    {
                        PropertyName: "*",
                        SaveAction: "UpdateAdd"
                    }
                ]
            };

            var result = prox.updateItem("Subscriber", props, options);

            return {
                success: result.Status === "OK",
                status: result.Status,
                message:
                    result.Status === "OK"
                        ? "Subscriber globally subscribed successfully"
                        : "Global subscribe failed",
                details: result.Results
            };
        } catch (ex) {
            return {
                success: false,
                status: "Error",
                message: "Exception occurred",
                error: ex.toString()
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