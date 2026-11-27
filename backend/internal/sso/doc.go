// Package sso is enterprise single sign-on for workspaces (#868): a workspace
// admin connects the organisation's identity provider (OpenID Connect or
// SAML 2.0) for one or more verified email domains, and members sign in
// through it instead of Google.
//
// The flow is browser-agnostic JSON, because the frontend proxy forwards
// API calls server to server and does not pass redirects through:
//
//  1. Start(email) finds the connection for the email's domain and returns
//     the identity provider URL to send the browser to. The OIDC state,
//     nonce and PKCE verifier, or the SAML request ID, are kept server side
//     for ten minutes and can be used once.
//  2. The identity provider sends the browser back to the site's callback
//     page, which posts what it received to FinishOIDC or FinishSAML.
//  3. A verified login returns a session token ("ops1." + payload + HMAC).
//     The frontend sends it as X-User-Token, like a Google ID token; the
//     Verifier accepts it while the connection stays enabled, the email's
//     domain stays verified on it, and the user is still a member of the
//     connection's workspace.
//
// Safety properties, each with a test:
//   - a connection signs nobody in until every domain on it is verified by a
//     DNS TXT record, so a workspace cannot claim another company's domain;
//     public mail domains cannot be claimed at all;
//   - an email from a domain the connection does not hold is refused;
//   - an SSO session for an email that is a member of another workspace is
//     refused, so one workspace's identity provider cannot open another's;
//   - OIDC: issuer, audience, expiry, signature and nonce are checked, the
//     code exchange uses PKCE, and the state is single use;
//   - SAML: the response or assertion must be signed by a configured
//     certificate; audience, destination, time window and InResponseTo are
//     checked, the relay state is single use and an assertion ID is accepted
//     once (replay);
//   - issuer discovery refuses loopback, private and link-local addresses
//     (no SSRF into the metadata server);
//   - stored OIDC client secrets are sealed with AES-GCM and never returned.
package sso
