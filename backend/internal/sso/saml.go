package sso

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"strings"
	"time"

	saml2 "github.com/russellhaering/gosaml2"
	"github.com/russellhaering/gosaml2/types"
	dsig "github.com/russellhaering/goxmldsig"

	"github.com/kendramoss/wassup-hackathon-2026/backend/internal/teams"
)

// maxSAMLResponse caps a posted SAMLResponse (base64).
const maxSAMLResponse = 256 << 10

const nameIDEmail = "urn:oasis:names:tc:SAML:1.1:nameid-format:emailAddress"

// emailAttributes are the attribute names identity providers use for email.
var emailAttributes = []string{
	"email", "mail", "Email", "emailAddress",
	"http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress",
	"urn:oid:0.9.2342.19200300.100.1.3",
}

func (s *Service) serviceProvider(c Connection) (*saml2.SAMLServiceProvider, error) {
	certs, err := ParseCerts(c.SAMLCertsPEM)
	if err != nil {
		return nil, err
	}
	return &saml2.SAMLServiceProvider{
		IdentityProviderSSOURL:      c.SAMLSSOURL,
		IdentityProviderSSOBinding:  saml2.BindingHttpRedirect,
		IdentityProviderIssuer:      c.SAMLIssuer,
		AssertionConsumerServiceURL: s.ACSURL(),
		ServiceProviderIssuer:       s.EntityID(),
		AudienceURI:                 s.EntityID(),
		IDPCertificateStore:         &dsig.MemoryX509CertificateStore{Roots: certs},
		NameIdFormat:                nameIDEmail,
		AllowMissingAttributes:      true,
		Clock:                       dsig.NewFakeClockAt(s.now()),
		MaximumDecompressedBodySize: 1 << 20,
	}, nil
}

func (s *Service) startSAML(c Connection, st *signinState) (string, error) {
	sp, err := s.serviceProvider(c)
	if err != nil {
		return "", err
	}
	doc, err := sp.BuildAuthRequestDocumentNoSig()
	if err != nil {
		return "", err
	}
	st.SAMLRequest = doc.Root().SelectAttrValue("ID", "")
	if st.SAMLRequest == "" {
		return "", errors.New("authn request has no ID")
	}
	return sp.BuildAuthURLRedirect(st.Key, doc)
}

// SPMetadata returns the service provider metadata XML for identity
// provider setup (entity ID, ACS URL, NameID format).
func (s *Service) SPMetadata() ([]byte, error) {
	sp := &saml2.SAMLServiceProvider{
		AssertionConsumerServiceURL: s.ACSURL(),
		ServiceProviderIssuer:       s.EntityID(),
		AudienceURI:                 s.EntityID(),
		NameIdFormat:                nameIDEmail,
		Clock:                       dsig.NewRealClock(),
	}
	md, err := sp.Metadata()
	if err != nil {
		return nil, err
	}
	return xml.MarshalIndent(md, "", "  ")
}

// samlLogin is a verified SAML assertion.
type samlLogin struct {
	email       string
	assertionID string
	expires     time.Time
}

func (s *Service) finishSAML(c Connection, st signinState, encoded string) (samlLogin, error) {
	encoded = strings.TrimSpace(encoded)
	if encoded == "" || len(encoded) > maxSAMLResponse {
		return samlLogin{}, errors.New("missing or oversized SAMLResponse")
	}
	sp, err := s.serviceProvider(c)
	if err != nil {
		return samlLogin{}, err
	}
	resp, err := sp.ValidateEncodedResponse(encoded)
	if err != nil {
		return samlLogin{}, fmt.Errorf("response: %w", err)
	}
	// Only a response to the request this browser started (no unsolicited,
	// IdP-initiated logins: they cannot be tied to a sign-in attempt).
	if resp.InResponseTo == "" || resp.InResponseTo != st.SAMLRequest {
		return samlLogin{}, errors.New("InResponseTo does not match the sign-in request")
	}
	if len(resp.Assertions) != 1 {
		return samlLogin{}, fmt.Errorf("want exactly one assertion, got %d", len(resp.Assertions))
	}
	a := resp.Assertions[0]
	if a.ID == "" {
		return samlLogin{}, errors.New("assertion has no ID")
	}
	if a.Conditions == nil || len(a.Conditions.AudienceRestrictions) == 0 {
		return samlLogin{}, errors.New("assertion has no audience restriction")
	}
	warn, err := sp.VerifyAssertionConditions(&a)
	if err != nil {
		return samlLogin{}, fmt.Errorf("conditions: %w", err)
	}
	if warn.InvalidTime {
		return samlLogin{}, errors.New("assertion is outside its validity window")
	}
	if warn.NotInAudience {
		return samlLogin{}, errors.New("assertion is for another audience")
	}
	if sc := a.Subject.SubjectConfirmation; sc != nil && sc.SubjectConfirmationData != nil {
		if irt := sc.SubjectConfirmationData.InResponseTo; irt != "" && irt != st.SAMLRequest {
			return samlLogin{}, errors.New("subject confirmation InResponseTo does not match")
		}
	}
	email := samlEmail(a)
	if email == "" {
		return samlLogin{}, errors.New("assertion has no usable email")
	}
	exp, err := time.Parse(time.RFC3339, a.Conditions.NotOnOrAfter)
	if err != nil {
		return samlLogin{}, errors.New("assertion NotOnOrAfter does not parse")
	}
	return samlLogin{email: email, assertionID: a.ID, expires: exp}, nil
}

func samlEmail(a types.Assertion) string {
	if a.Subject != nil && a.Subject.NameID != nil {
		if e, ok := teams.NormalEmail(a.Subject.NameID.Value); ok {
			return e
		}
	}
	if a.AttributeStatement == nil {
		return ""
	}
	for _, want := range emailAttributes {
		for _, at := range a.AttributeStatement.Attributes {
			if at.Name != want && at.FriendlyName != want {
				continue
			}
			for _, v := range at.Values {
				if e, ok := teams.NormalEmail(v.Value); ok {
					return e
				}
			}
		}
	}
	return ""
}

// replayKey is the one-time key for an accepted assertion.
func replayKey(conn, assertionID string) string { return "saml-assertion:" + conn + ":" + assertionID }

func (s *Service) markAssertion(ctx context.Context, conn string, l samlLogin) error {
	ttl := l.expires.Sub(s.now()) + 5*time.Minute
	if ttl < time.Minute {
		ttl = time.Minute
	}
	if ttl > MaxSessionTTL {
		ttl = MaxSessionTTL
	}
	if err := s.state.Put(ctx, replayKey(conn, l.assertionID), []byte{1}, ttl); err != nil {
		if errors.Is(err, ErrExists) {
			return errors.New("assertion was already used (replay)")
		}
		return err
	}
	return nil
}
