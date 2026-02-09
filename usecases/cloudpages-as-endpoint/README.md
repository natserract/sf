# CloudPages as Endpoint

Salesforce’s CloudPages feature lets you build and host landing pages and content directly in Marketing Cloud. It includes forms, content blocks, and dynamic scripting with AMPscript/SSJS

One way to expose an endpoint-like URL inside SFMC is to use a CloudPages Code Resource instead of a standard landing page. A Code Resource creates a simple publicly-accessible URL which can return CSS, JavaScript, JSON, or text and can include server-side logic via SSJS/AMPscript.

## Pros 
- No extra hosting cost
- Native access to: Data Extensions, Contact, Journey, Email triggers (Via SSJS)
- Using Code Resources under Cloud Pages, doesn't consume additional Super Messages.

## CloudPages limitation
- API Call Limits: While SSJS can use WSProxy for faster data operations, external REST/SOAP API calls are limited. Generally, non-tracking operations have a 120-second timeout, while data retrieval is 300 seconds.
- What they DO impact, is the number of consumed Super Messages, which is a billable unit, similar to your emails. Each Cloud Page impression equals 1 Super Message. (Only for landing page, need confirm to support) 
- API Limits: While landing pages don't inherently consume API calls, if they trigger SSJS/AMPscript to call the API, they are subject to **standard daily and concurrent API limits**.
- Performance & Rendering: Complex scripts (AMPscript/SSJS) or excessive data lookups (LookupRows()) can cause pages to load slowly or time out.
- Can't implement retryable

## Notes
- WSProxy doesn't requires Auth, client id or secret

## Implementation

Use this template
```js
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
```

## Running

Trigger this endpoint:
```bash
POST /xxx HTTP/1.1
Host: xxx.pub.sfmc-content.com
Content-Type: application/json
Content-Length: 17

{
    "username": "natserract"
}
```