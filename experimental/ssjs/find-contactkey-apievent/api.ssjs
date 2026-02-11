<script runat="server">
    Platform.Load("core", "1");

    var API_KEYS_DE = "API_Keys";
    var ACCOUNT_ID = 526001350;

    ///////// Main API Handler
    try {
        var apiKey = getApiKey();
        var validation = validateApiKey(apiKey);

        var requestData;
        var postData = Platform.Request.GetPostData(); // Try to get POST data first

        // Check API KEY first
        if (!validation.valid) {
            returnError(401, validation.reason);
            return;
        }

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

        // Validate required field: email
        if (!requestData.email) {
            returnError(400, "Missing required field: email");
            return;
        }
        // Validate eventDefinitionKey
        if (!requestData.eventDefinitionKey) {
            returnError(400, "Missing required field: eventDefinitionKey");
            return;
        }

        var email = requestData.email;
        var eventDefinitionKey = requestData.eventDefinitionKey;
        var eventData = requestData.eventData || {};

        // Step 1: Get Auth Token
        var accessTokenRes = getAuthToken();
        if (!accessTokenRes.valid) {
            returnError(500, {
                success: false,
                error: accessTokenRes.reason
            });
            return;
        }

        var accessToken = accessTokenRes.token;

        // Step 2: Find contact key by email
        var contactKeyRes = findContactKeyByEmail(email, accessToken);
        if (!contactKeyRes.valid) {
            returnError(404, {
                success: false,
                error: "Contact not found for email: " + email
            });
            return;
        }

        var contactKey = contactKeyRes.contactKey;

        // Step 3: Trigger API Event
        var eventResult = triggerApiEvent(contactKey, eventDefinitionKey, email, eventData, accessToken);
        if (!eventResult.valid) {
            returnError(500, {
                success: false,
                error: eventResult.reason
            });
            return;
        }

        returnSuccess(200, {
            success: true,
            message: eventResult.eventInstanceId
        });
    } catch (ex) {
        returnError(500, "Server error: " + ex.toString());
    }

    ///////// API FUNCTIONS
    function getAuthToken() {
        try {
            var apiKey = getApiKey();

            var apiKeysRows = Platform.Function.LookupRows(
                API_KEYS_DE,
                "APIKey", // Column to match
                apiKey // Value to match
            );
            if (!apiKeysRows || apiKeysRows.length === 0) {
                return {
                    valid: false,
                    reason: "No active API credentials found in API_Keys data extension"
                };
            }

            var apiKeys = apiKeysRows[0];
            if (!apiKeys.ClientID || !apiKeys.ClientSecret) {
                return {
                    valid: false,
                    reason: "Missing ClientID or ClientSecret in API_Keys data extension"
                };
            }

            // Make auth request
            var authEndpoint =
                "https://xx.auth.marketingcloudapis.com/v2/token";

            var payload = {
                grant_type: "client_credentials",
                client_id: apiKeys.ClientID,
                client_secret: apiKeys.ClientSecret,
                account_id: ACCOUNT_ID
            };

            var result = HTTP.Post(
                authEndpoint,
                "application/json",
                Stringify(payload),
                [],
                []
            );

            if (result.StatusCode == 200) {
                var parsedResponse = Platform.Function.ParseJSON(
                    result.Response[0]
                );

                return {
                    valid: true,
                    token: String(parsedResponse.access_token)
                };
            } else {
                return {
                    valid: false,
                    reason: "Error: " + String(response.content)
                };
            }
        } catch (ex) {
            return {
                valid: false,
                reason: ex.toString()
            };
        }
    }

    function findContactKeyByEmail(email, accessToken) {
        try {
            var endpoint =
                "https://xx.rest.marketingcloudapis.com/contacts/v1/addresses/email/search";

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
            var endpoint =
                "https://xx.rest.marketingcloudapis.com/interaction/v1/events";

            // Prepare event data
            var data = {
                contact_key: contactKey,
                email: email,
                fullname: eventData.fullname || "",
                slab_name: eventData.slab_name || "",
                loyalty_points: eventData.loyalty_points || 0
            };

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
                    reason:
                        "API Event Error - Response: " + String(parsedResponse)
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
    // Validate API key with comprehensive checks
    function validateApiKey(apiKey) {
        if (!apiKey) {
            return {
                valid: false,
                reason: "Invalid API key format"
            };
        }

        try {
            // Use Platform.Function.Lookup instead
            var deRows = Platform.Function.LookupRows(
                API_KEYS_DE, // Data Extension External Key
                "APIKey", // Column to match
                apiKey // Value to match
            );

            if (!deRows || deRows.length === 0) {
                return {
                    valid: false,
                    reason: "API key not found"
                };
            }

            var keyData = deRows[0];

            // Check if key is active
            if (
                !keyData.IsActive ||
                keyData.IsActive === false ||
                keyData.IsActive === "false"
            ) {
                return {
                    valid: false,
                    reason: "API key is inactive"
                };
            }

            return {
                valid: true
            };
        } catch (ex) {
            return {
                valid: false,
                reason: "Error validating API key: " + ex.toString()
            };
        }
    }

    function getApiKey() {
        var apiKeyHeader = Platform.Request.GetRequestHeader("X-API-Key");
        if (apiKeyHeader) {
            return apiKeyHeader;
        }

        return null;
    }

    /**
     * Helper function to return success response
     */
    function returnSuccess(statusCode, data) {
        Platform.Response.SetResponseHeader("Content-Type", "application/json");
        Platform.Response.SetResponseHeader("Access-Control-Allow-Origin", "*");
        Platform.Response.SetResponseHeader(
            "Access-Control-Allow-Methods",
            "GET, POST, OPTIONS"
        );
        Platform.Response.SetResponseHeader(
            "Access-Control-Allow-Headers",
            "Content-Type"
        );
        Write(Stringify(data));
    }

    /**
     * Helper function to return error response
     */
    function returnError(statusCode, message) {
        Platform.Response.SetResponseHeader("Content-Type", "application/json");
        Platform.Response.SetResponseHeader("Access-Control-Allow-Origin", "*");
        Write(
            Stringify({
                success: false,
                error: message
            })
        );
    }
</script>