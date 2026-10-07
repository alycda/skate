# Smoke test: does the auth server accept your development token? Needs real
# credentials, so run it by hand once you have them; CI does not.
#
#   hurl --test \
#     --variable auth_url=https://<your Auth URL from the Portal> \
#     --variable database_id=<your Database ID> \
#     --variable token=<your development token> \
#     test/hurl/ditto-auth-login.hurl
#
# The provider below is what the Go SDK's DevelopmentAuthenticationProvider()
# returns, and it is what `skate sync` logs in with. Edit it here to try another
# provider.
#
# Caveat: the HTTP login endpoint is documented for JWTs issued through an auth
# webhook ("Ideal for integrations with temporary session tokens"). The docs do
# not say whether it also accepts the development provider. If this fails on a
# token that `skate sync` logs in with, that may be the endpoint, not your
# setup; check `SKATE_DITTO_LOG_LEVEL=debug skate sync --timeout 20s` too.
#
# Source: https://docs.ditto.live/cloud/http-api/auth-and-params
POST {{auth_url}}/_ditto/auth/http_login
[FormParams]
appId: {{database_id}}
token: {{token}}
provider: __playgroundProvider
HTTP 200
[Asserts]
jsonpath "$.httpToken" isString
jsonpath "$.tokenExpiry" isString
