<script runat="server">
    Platform.Load("core", "1");

    // Setup
    HTTPHeader.SetValue("Content-Type", "application/json");

    var API_KEYS_DE = "API_Keys";

    ///////// Main API Handler
    try {
      var apiKey = getApiKey();
      var validation = validateApiKey(apiKey);

      var requestData;
      var postData = Platform.Request.GetPostData(); // Try to get POST data first

      // Check API KEY first
      if (!validation.valid) {
        sendResponse("401 Unauthorized", {
          success: false,
          error: validation.reason
        });
        return;
      }

      // Ensure payload doesn't empty
      if (postData && postData !== "") {
        // POST request
        try {
          requestData = Platform.Function.ParseJSON(postData);
        } catch (ex) {
          sendResponse("400 Bad Request", {
            success: false,
            error: "Invalid JSON in request body"
          });
          return;
        }
      } else {
        sendResponse("400 Bad Request", {
          success: false,
          error: "Payload was empty"
        });
        return;
      }

      // Get action type
      var action = requestData.action || "";
      var subscriberKey = requestData.subscriberKey || "";
      var emailAddress = requestData.emailAddress || "";

      // Validate required fields
      if (!subscriberKey || !emailAddress) {
        sendResponse("400 Bad Request", {
          success: false,
          error: "subscriberKey and emailAddress are required"
        });
        return;
      }

      // Validate action parameter
      if (!action) {
        sendResponse("400 Bad Request", {
          success: false,
          error: "action parameter is required (unsubscribe or subscribe)"
        });
        return;
      }

      // Route to appropriate function based on action
      var result;
      switch (action.toLowerCase()) {
        case "unsubscribe":
          result = globalUnsubscribe(subscriberKey, emailAddress);
          break;

        case "subscribe":
          result = globalSubscribe(subscriberKey, emailAddress);
          break;

        default:
          sendResponse("400 Bad Request", {
            success: false,
            error: "Invalid action. Allowed values: 'unsubscribe' or 'subscribe'"
          });
          return;
      }

      // Send response
      if (result.success) {
        sendResponse("200 OK", result);
      } else {
        sendResponse("500 Internal Server Error", result);
      }
    } catch (ex) {
      sendResponse("500 Internal Server Error", {
        success: false,
        error: "Server error: " + ex.toString()
      });
    }

    ///////// UTILS
    function sendResponse(statusCode, data) {
      HTTPHeader.SetValue("Status", statusCode);
      Write(Stringify(data));
    }

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

    // Function for global unsubscribe
    function globalUnsubscribe(subscriberKey, emailAddress) {
      try {
        var prox = new Script.Util.WSProxy();

        var props = {
          SubscriberKey: subscriberKey,
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
    function globalSubscribe(subscriberKey, emailAddress) {
      try {
        var prox = new Script.Util.WSProxy();

        var props = {
          SubscriberKey: subscriberKey,
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
</script>