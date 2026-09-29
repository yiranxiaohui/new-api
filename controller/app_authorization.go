package controller

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// App authorization lets a native app (for example a desktop client) obtain an
// API token without handling the user's account credentials. It follows the
// OAuth 2.0 authorization code flow for native apps (RFC 8252) with mandatory
// PKCE (RFC 7636, S256):
//
//  1. The app opens /app-auth in the system browser with a loopback
//     redirect_uri, a code_challenge and a state value.
//  2. The signed-in user approves on the consent page, which calls
//     POST /api/app-auth/authorize. A new API token is created and a one-time
//     authorization code bound to it is issued.
//  3. The browser is redirected to the loopback redirect_uri with the code.
//  4. The app calls POST /api/app-auth/token with the code and the
//     code_verifier and receives the token key.
//
// Codes are single use, expire after appAuthorizationCodeTTL, are stored only
// as HMAC digests in auth_flows, and stay bound to the approving login session.
//
// Apps that manage the account itself (balance, API tokens) request the
// "account" scope instead: the consent page calls
// POST /api/app-auth/authorize/account, no API token is created, and the code
// exchanges for a new login session of the app's own (an access token plus a
// refresh token the app presents as the refresh cookie). The session is listed
// with the user's other login sessions and can be revoked there.

const (
	appAuthorizationCodeTTL       = 5 * time.Minute
	appAuthorizationMaxClientName = 64
	appAuthorizationMaxState      = 512
	appAuthorizationMaxRedirect   = 512

	// appAuthorizationScopeAccount grants a login session instead of an API token.
	appAuthorizationScopeAccount = "account"
	// appSignInLoginMethod is the login method recorded for app sessions.
	appSignInLoginMethod = "app"
)

// appAuthorizationScopes lists the scopes this server supports, for /api/status.
var appAuthorizationScopes = []string{"token", appAuthorizationScopeAccount}

type appAuthorizationRequest struct {
	ClientName          string       `json:"client_name"`
	RedirectURI         string       `json:"redirect_uri"`
	CodeChallenge       string       `json:"code_challenge"`
	CodeChallengeMethod string       `json:"code_challenge_method"`
	State               string       `json:"state"`
	Token               tokenRequest `json:"token"`
}

// appSignInRequest asks for a login session (the "account" scope).
type appSignInRequest struct {
	ClientName          string `json:"client_name"`
	RedirectURI         string `json:"redirect_uri"`
	CodeChallenge       string `json:"code_challenge"`
	CodeChallengeMethod string `json:"code_challenge_method"`
	State               string `json:"state"`
}

type appAuthorizationExchangeRequest struct {
	Code         string `json:"code"`
	CodeVerifier string `json:"code_verifier"`
	RedirectURI  string `json:"redirect_uri"`
}

// appAuthorizationGrant is the server-side state behind an authorization code.
type appAuthorizationGrant struct {
	// Scope is empty for API token grants and "account" for sign-in grants.
	Scope         string `json:"scope,omitempty"`
	TokenID       int    `json:"token_id,omitempty"`
	ClientName    string `json:"client_name"`
	RedirectURI   string `json:"redirect_uri"`
	CodeChallenge string `json:"code_challenge"`
	// AuthVersion is the user's auth version at approval (sign-in grants), so a
	// password change or "sign out everywhere" in between voids the code.
	AuthVersion int64 `json:"auth_version,omitempty"`
}

// normalizeAppClientName returns the trimmed display name of the requesting app,
// or false when it is empty, too long, or contains control characters.
func normalizeAppClientName(value string) (string, bool) {
	name := strings.TrimSpace(value)
	if name == "" || !utf8.ValidString(name) || utf8.RuneCountInString(name) > appAuthorizationMaxClientName {
		return "", false
	}
	for _, r := range name {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
			return "", false
		}
	}
	return name, true
}

// isLoopbackRedirectURI accepts only RFC 8252 loopback redirects:
// http://127.0.0.1:<port>/<path> or http://[::1]:<port>/<path>, without user
// info, query or fragment. Codes can therefore only be delivered to a process
// on the user's own machine.
func isLoopbackRedirectURI(value string) bool {
	if value == "" || len(value) > appAuthorizationMaxRedirect {
		return false
	}
	redirect, err := url.Parse(value)
	if err != nil || redirect.Scheme != "http" || redirect.User != nil || redirect.Opaque != "" ||
		redirect.RawQuery != "" || redirect.ForceQuery || redirect.Fragment != "" || strings.Contains(value, "#") {
		return false
	}
	host, port, err := net.SplitHostPort(redirect.Host)
	if err != nil || (host != "127.0.0.1" && host != "::1") {
		return false
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 || strconv.Itoa(portNumber) != port {
		return false
	}
	return redirect.Path == "" || strings.HasPrefix(redirect.Path, "/")
}

// isPKCEVerifier checks the RFC 7636 code_verifier syntax: 43-128 characters
// from the unreserved set.
func isPKCEVerifier(value string) bool {
	if len(value) < 43 || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if !(r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '.' || r == '_' || r == '~') {
			return false
		}
	}
	return true
}

func pkceS256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// AuthorizeApp is called by the consent page after the signed-in user approves
// an app. It creates the requested API token and returns the loopback redirect
// URL carrying a one-time authorization code.
func AuthorizeApp(c *gin.Context) {
	setAuthNoStore(c)
	if !common.AppAuthorizationEnabled {
		common.ApiErrorI18n(c, i18n.MsgAppAuthDisabled)
		return
	}
	// Consent must come from a browser login session, not a personal access token.
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": i18n.T(c, i18n.MsgAuthNotLoggedIn)})
		return
	}
	var request appAuthorizationRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	clientName, valid := validAppAuthorizationRequest(request.ClientName, request.RedirectURI, request.CodeChallenge, request.CodeChallengeMethod, request.State)
	if !valid {
		common.ApiErrorI18n(c, i18n.MsgAppAuthInvalidRequest)
		return
	}
	params := tokenAuditParams(c)
	params["client_name"] = clientName

	token, created := createUserToken(c, request.Token)
	if !created {
		return
	}
	payload, err := common.Marshal(appAuthorizationGrant{
		TokenID:       token.Id,
		ClientName:    clientName,
		RedirectURI:   request.RedirectURI,
		CodeChallenge: request.CodeChallenge,
	})
	var code string
	if err == nil {
		code, _, err = model.CreateAuthFlow(model.AuthFlowCreate{
			Purpose:   model.AuthFlowPurposeAppAuthorization,
			UserId:    identity.UserID,
			SessionId: identity.SessionID,
			Payload:   string(payload),
			ExpiresAt: time.Now().Add(appAuthorizationCodeTTL),
		})
	}
	if err != nil {
		if deleteErr := model.DeleteTokenById(token.Id, identity.UserID); deleteErr != nil {
			common.SysError("failed to remove token after app authorization failure: " + deleteErr.Error())
		}
		writeSecurityOperationError(c, err)
		return
	}

	common.SetContextKey(c, constant.ContextKeyTokenAuditSucceeded, true)
	common.ApiSuccess(c, gin.H{"redirect_url": appAuthorizationRedirect(request.RedirectURI, code, request.State)})
}

// validAppAuthorizationRequest checks the parameters shared by every scope and
// returns the normalized app name.
func validAppAuthorizationRequest(clientName, redirectURI, codeChallenge, method, state string) (string, bool) {
	name, validName := normalizeAppClientName(clientName)
	challenge, err := base64.RawURLEncoding.DecodeString(codeChallenge)
	if !validName || !isLoopbackRedirectURI(redirectURI) || method != "S256" ||
		err != nil || len(challenge) != sha256.Size || len(state) > appAuthorizationMaxState {
		return "", false
	}
	return name, true
}

// appAuthorizationRedirect is the loopback URL carrying the code and state.
func appAuthorizationRedirect(redirectURI, code, state string) string {
	redirect, _ := url.Parse(redirectURI)
	query := url.Values{}
	query.Set("code", code)
	if state != "" {
		query.Set("state", state)
	}
	redirect.RawQuery = query.Encode()
	return redirect.String()
}

// AuthorizeAppSignIn is called by the consent page after the signed-in user
// lets an app sign in to the account (the "account" scope). No API token is
// created; the returned loopback URL carries a one-time code that exchanges
// for a new login session.
func AuthorizeAppSignIn(c *gin.Context) {
	setAuthNoStore(c)
	if !common.AppAuthorizationEnabled {
		common.ApiErrorI18n(c, i18n.MsgAppAuthDisabled)
		return
	}
	identity, ok := middleware.GetSessionAuthIdentity(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"success": false, "message": i18n.T(c, i18n.MsgAuthNotLoggedIn)})
		return
	}
	var request appSignInRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil {
		common.ApiErrorI18n(c, i18n.MsgInvalidParams)
		return
	}
	clientName, valid := validAppAuthorizationRequest(request.ClientName, request.RedirectURI, request.CodeChallenge, request.CodeChallengeMethod, request.State)
	if !valid {
		common.ApiErrorI18n(c, i18n.MsgAppAuthInvalidRequest)
		return
	}
	audit := map[string]any{"client_name": clientName, "success": false}
	defer func() { recordUserSecurityAudit(c, identity.UserID, "session.app_authorize", audit) }()

	payload, err := common.Marshal(appAuthorizationGrant{
		Scope:         appAuthorizationScopeAccount,
		ClientName:    clientName,
		RedirectURI:   request.RedirectURI,
		CodeChallenge: request.CodeChallenge,
		AuthVersion:   identity.UserAuthVersion,
	})
	var code string
	if err == nil {
		code, _, err = model.CreateAuthFlow(model.AuthFlowCreate{
			Purpose:   model.AuthFlowPurposeAppAuthorization,
			UserId:    identity.UserID,
			SessionId: identity.SessionID,
			Payload:   string(payload),
			ExpiresAt: time.Now().Add(appAuthorizationCodeTTL),
		})
	}
	if err != nil {
		writeSecurityOperationError(c, err)
		return
	}
	audit["success"] = true
	common.ApiSuccess(c, gin.H{"redirect_url": appAuthorizationRedirect(request.RedirectURI, code, request.State)})
}

// ExchangeAppAuthorization redeems an authorization code for the key of the
// API token the user approved. The code is consumed on every attempt that
// names a live code, including a wrong verifier, so it cannot be retried.
func ExchangeAppAuthorization(c *gin.Context) {
	setAuthNoStore(c)
	if !common.AppAuthorizationEnabled {
		common.ApiErrorI18n(c, i18n.MsgAppAuthDisabled)
		return
	}
	var request appAuthorizationExchangeRequest
	if err := common.DecodeJson(c.Request.Body, &request); err != nil || request.Code == "" || !isPKCEVerifier(request.CodeVerifier) {
		common.ApiErrorI18n(c, i18n.MsgAppAuthInvalidRequest)
		return
	}
	match := model.AuthFlowMatch{Purpose: model.AuthFlowPurposeAppAuthorization}
	flow, err := model.GetAuthFlow(request.Code, match)
	if err != nil {
		common.ApiErrorI18n(c, i18n.MsgAppAuthInvalidGrant)
		return
	}
	match.UserId, match.SessionId = flow.UserId, flow.SessionId
	var grant appAuthorizationGrant
	grantErr := common.UnmarshalJsonStr(flow.Payload, &grant)
	if grantErr == nil && grant.Scope != "" && grant.Scope != appAuthorizationScopeAccount {
		grantErr = errors.New("unknown app authorization scope")
	}
	signIn := grantErr == nil && grant.Scope == appAuthorizationScopeAccount
	action := "token.app_key_exchange"
	if signIn {
		action = "session.app_sign_in"
	}
	audit := map[string]any{"success": false}
	defer func() { recordUserSecurityAudit(c, flow.UserId, action, audit) }()

	if grantErr == nil {
		audit["client_name"] = grant.ClientName
		if !signIn {
			audit["id"] = grant.TokenID
		}
		expected := []byte(grant.CodeChallenge)
		if subtle.ConstantTimeCompare([]byte(pkceS256Challenge(request.CodeVerifier)), expected) != 1 ||
			subtle.ConstantTimeCompare([]byte(request.RedirectURI), []byte(grant.RedirectURI)) != 1 {
			grantErr = errors.New("app authorization verifier or redirect mismatch")
		}
	}
	if grantErr == nil {
		_, grantErr = service.ValidateSessionReference(flow.UserId, flow.SessionId)
	}
	if signIn {
		exchangeAppSignIn(c, request.Code, match, flow.UserId, grant, grantErr, audit)
		return
	}
	var token model.Token
	_, err = model.ConsumeAuthFlowWithAction(request.Code, match, func(tx *gorm.DB, _ *model.AuthFlow) error {
		if grantErr != nil {
			return nil
		}
		// A deleted token still burns the code; its zero status fails below.
		err := tx.Where("id = ? AND user_id = ?", grant.TokenID, flow.UserId).First(&token).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return err
	})
	if err != nil || grantErr != nil || token.Status != common.TokenStatusEnabled {
		common.ApiErrorI18n(c, i18n.MsgAppAuthInvalidGrant)
		return
	}

	data := gin.H{
		"key":   token.GetFullKey(),
		"token": gin.H{"id": token.Id, "name": token.Name, "group": token.Group},
	}
	if user, err := model.GetUserById(flow.UserId, false); err == nil {
		data["user"] = gin.H{"id": user.Id, "username": user.Username, "display_name": user.DisplayName}
	}
	audit["success"] = true
	common.ApiSuccess(c, data)
}

// exchangeAppSignIn burns a sign-in code and starts a new login session for
// the app. The refresh token is returned in the body because the app is not a
// browser; it presents it as the refresh cookie when renewing.
func exchangeAppSignIn(c *gin.Context, code string, match model.AuthFlowMatch, userID int, grant appAuthorizationGrant, grantErr error, audit map[string]any) {
	if _, err := model.ConsumeAuthFlowWithAction(code, match, func(*gorm.DB, *model.AuthFlow) error { return nil }); err != nil || grantErr != nil {
		common.ApiErrorI18n(c, i18n.MsgAppAuthInvalidGrant)
		return
	}
	bundle, err := service.CreateLoginSessionAtAuthVersion(userID, grant.AuthVersion, appSignInLoginMethod, c.ClientIP(), c.Request.UserAgent())
	if err != nil {
		if errors.Is(err, service.ErrLoginSessionRevoked) || errors.Is(err, service.ErrLoginSessionInvalid) {
			common.ApiErrorI18n(c, i18n.MsgAppAuthInvalidGrant)
			return
		}
		writeAuthSessionError(c, err)
		return
	}
	user, err := model.GetSelfUserById(userID)
	if err != nil {
		_, _ = model.RevokeUserSession(userID, bundle.Session.SID, "app_sign_in_failed")
		common.ApiError(c, err)
		return
	}
	audit["success"] = true
	model.UpdateUserLastLoginAt(userID)
	common.ApiSuccess(c, gin.H{
		"scope":             appAuthorizationScopeAccount,
		"access_token":      bundle.AccessToken,
		"token_type":        bundle.TokenType,
		"access_expires_at": bundle.AccessExpiresAt,
		"refresh_token":     bundle.RefreshToken,
		"session":           bundle.Session,
		"user":              buildSelfUserData(user),
	})
}
