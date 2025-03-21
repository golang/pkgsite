#!/usr/bin/env bash

# Display the state of the Cloud Armor rules for the prod frontend.

echo 'Cloud Armor rule for prod. Note rateLimitOptions.'

gcloud compute security-policies describe prod-frontend

echo
echo 'To modify:'
echo '- Obtain the necessary permissions.'
echo '- Visit https://console.google.com/net-security/securitypolicies/details/prod-frontend?project=$PROJECT&hl=en&tab=rules'
echo '- Click Edit'
