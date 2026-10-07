# Smoke test: is the Ditto auth server reachable, and does it answer the
# documented login endpoint? Needs no token, so it runs before you have keys.
#
#   hurl --test \
#     --variable auth_url=https://<your Auth URL from the Portal> \
#     --variable database_id=<your Database ID> \
#     test/hurl/ditto-auth-reachable.hurl
#
# The request is a login with a token that cannot be valid, so a healthy server
# should refuse it. What this checks, and what it does not:
#
#   - It checks that DNS, TLS and HTTP to the auth URL work, and that the
#     refusal comes back as a 4xx with the documented {"error": ...} body.
#   - It does not check that your token is accepted (see ditto-auth-login.hurl),
#     or that sync works.
#
# Source: https://docs.ditto.live/cloud/http-api/auth-and-params documents the
# endpoint and form fields, and says errors carry a status code and a JSON
# object with a single "error" key. It does not list the status codes, so the
# 4xx range below is an assumption: if a healthy server answers otherwise,
# loosen it here rather than reading it as a setup problem.
POST {{auth_url}}/_ditto/auth/http_login
[FormParams]
appId: {{database_id}}
token: skate-smoke-test-not-a-real-token
provider: skate-smoke-test
HTTP *
[Asserts]
status >= 400
status < 500
jsonpath "$.error" exists
