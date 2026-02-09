#!/bin/bash

# A script to automate the generation of a long-lived Dropbox access token
# and create the dropbox.json file for the application.

# --- Step 0: Check for jq dependency ---
if ! command -v jq &>/dev/null; then
    echo "❌ ERROR: 'jq' is not installed, but is required for this script to run."
    echo "Please install it to continue (e.g., 'sudo apt-get install jq' or 'brew install jq')."
    exit 1
fi

# --- Step 1: Clear the screen and show instructions ---
clear
echo "Dropbox Token Setup Helper"
echo "---------------------------------"
echo "This script will:"
echo "  1. Help you get your permanent Dropbox refresh and access tokens."
echo "  2. Create the 'dropbox.json' file in the correct format."
echo ""
echo "Before you begin, make sure you have visited the Dropbox authorization URL"
echo "in your browser to get a temporary authorization code."
echo ""

# --- Step 2: Prompt for user input ---
echo "Please enter the following information from your Dropbox App Console:"
read -p "Enter your App Key: " APP_KEY
read -s -p "Enter your App Secret: " APP_SECRET # -s flag hides the input
echo ""
echo ""
echo "Now, please enter the temporary code from the browser URL (it's the value of the 'code' or 'auth_code' parameter):"
read -p "Enter your temporary Authorization Code: " AUTH_CODE
echo ""

# --- Step 3: Validate input ---
if [ -z "$APP_KEY" ] || [ -z "$APP_SECRET" ] || [ -z "$AUTH_CODE" ]; then
    echo "❌ ERROR: One or more required values were not entered. Please run the script again."
    exit 1
fi

# --- Step 4: Execute the curl command ---
echo "✅ Thank you. Requesting the permanent token from Dropbox..."
echo "---------------------------------"

# We store the response from curl in a variable
# This version does NOT include the redirect_uri, as that was causing issues.
RESPONSE=$(curl --silent --show-error --location https://api.dropboxapi.com/oauth2/token \
    -d code="$AUTH_CODE" \
    -d grant_type=authorization_code \
    -u "$APP_KEY:$APP_SECRET")

# --- Step 5: Check for errors ---
if [ $? -ne 0 ]; then
    echo "❌ ERROR: The curl command failed. Please check your network connection."
    exit 1
fi

if echo "$RESPONSE" | grep -q "error_summary"; then
    echo "❌ ERROR: Dropbox returned an error. Here is the full response:"
    echo "$RESPONSE" | jq . # Pretty-print the error
    echo ""
    echo "Please check that your App Key, Secret, and Authorization Code are correct and that the code has not expired."
    exit 1
fi

echo "🎉 SUCCESS! Tokens received from Dropbox."
echo ""

# --- Step 6: Create the dropbox.json file ---
echo "Creating the 'dropbox.json' file..."

# Use jq to parse the response and construct the new JSON object.
# The '.' means "take the whole input object".
# We then create a new object with only the two keys we need.
jq '{access_token: .access_token, refresh_token: .refresh_token}' <<<"$RESPONSE" >dropbox.json

if [ $? -ne 0 ]; then
    echo "❌ ERROR: Failed to create 'dropbox.json'. Here is the raw response from Dropbox:"
    echo "$RESPONSE"
    exit 1
fi

echo ""
echo "---------------------------------"
echo "✅ DONE! A file named 'dropbox.json' has been created in this directory."
echo ""
echo "Next Steps:"
echo "1. Create a 'data' directory if you haven't already ('mkdir data')."
echo "2. Move the new file into it ('mv dropbox.json data/')."
echo "3. You are now ready to run your Docker container!"
echo "---------------------------------"
