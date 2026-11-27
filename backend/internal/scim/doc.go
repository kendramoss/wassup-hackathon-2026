// Package scim is SCIM 2.0 user provisioning for workspaces (#869, RFC 7643
// and RFC 7644, Users only): the organisation's identity provider adds,
// changes and removes workspace members.
//
// A workspace admin issues one SCIM token for the workspace, shown once and
// stored only as a SHA-256 hash, and names the email domains it manages.
// Every domain must be verified on one of the workspace's SSO connections
// (#868), so a token can never provision someone else's employees.
//
// What a token can do, each with a test:
//   - see and change only members of its own workspace whose email is on one
//     of its domains; anyone else is invisible (404, absent from lists);
//   - add a person who is in no workspace (one in another workspace is a 409
//     that does not name it);
//   - never make, demote or remove an admin unless it was issued with
//     allow_admin, and never remove the last admin either way;
//   - nothing once revoked or replaced; a wrong token is refused before any
//     lookup is made with its value.
package scim
