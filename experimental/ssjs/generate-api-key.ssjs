<script runat="server">
  Platform.Load("core", "1");

  // Set response headers for JSON API
  HTTPHeader.SetValue("Content-Type", "application/json");

  // Configuration
  var API_KEYS_DE = "API_Keys";

  // Function to send JSON response
  function sendResponse(statusCode, data) {
    HTTPHeader.SetValue("Status", statusCode);
    Write(Stringify(data));
  }

  // Generate a random API key
  function generateApiKey() {
    var chars = 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789';
    var apiKey = 'sk_live_';
    
    // Generate 40 character random key
    for (var i = 0; i < 40; i++) {
      var randomIndex = Math.floor(Math.random() * chars.length);
      apiKey += chars.charAt(randomIndex);
    }
    
    return apiKey;
  }

  // Save API key to Data Extension
  function saveApiKey() {
    try {
      var apiKey = generateApiKey();
      var de = DataExtension.Init(API_KEYS_DE);
      
      // Add new API key to Data Extension
      de.Rows.Add({
        APIKey: apiKey,
        IsActive: true
      });
      
      return {
        success: true,
        apiKey: apiKey,
        message: "API key generated and stored successfully"
      };
      
    } catch (ex) {
      return {
        success: false,
        error: "Error: " + ex.toString()
      };
    }
  }

  // Main handler
  try {
    var requestData;
    var postData = Platform.Request.GetPostData();
    
    if (postData && postData !== '') {
      try {
        requestData = Platform.Function.ParseJSON(postData);
      } catch (ex) {
        sendResponse("400 Bad Request", {
          success: false,
          error: 'Invalid JSON'
        });
        return;
      }
    } else {
      sendResponse("400 Bad Request", {
        success: false,
        error: 'Request body is required'
      });
      return;
    }
    
    var result = saveApiKey();
    
    if (result.success) {
      sendResponse("200 OK", result);
    } else {
      sendResponse("400 Bad Request", result);
    }
    
  } catch (ex) {
    sendResponse("500 Internal Server Error", {
      success: false,
      error: 'Server error: ' + ex.toString()
    });
  }
</script>