<script runat="server">
  Platform.Load("core", "1");

  // Set response headers for JSON API
  HTTPHeader.SetValue("Content-Type", "application/json");

  // Function to send JSON response
  function sendResponse(statusCode, data) {
    HTTPHeader.SetValue("Status", statusCode);
    Write(Stringify(data));
  }

  // Main API Handler
  // Method: POST
  try {
    var requestData;

    // Get post data
    var postData = Platform.Request.GetPostData();

    // Validate the payload
    if (postData && postData !== '') {
      // POST request
      try {
        requestData = Platform.Function.ParseJSON(postData);
      } catch (ex) {
        sendResponse("400 Bad Request", {
          success: false,
          error: 'Invalid JSON in request body'
        });
        return;
      }
    }

  // Get payload
  var username = requestData.username || '';

  // Validate required fields
  if (!username) {
    sendResponse("400 Bad Request", {
      success: false,
      error: 'username are required'
    });
    return;
  }

  // Route to appropriate function based on action
  var result;

  // Sample success response
  result =   {
    success: true,
    status: 'OK',
    data: 1
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
      error: 'Server error: ' + ex.toString()
    });
}
</script>