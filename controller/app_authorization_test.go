package controller

import (
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/i18n"
	"github.com/QuantumNous/new-api/middleware"
	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/service"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	appAuthTestVerifier    = "pier-test-verifier-0123456789abcdefghijklmnopqrstuvwxyz"
	appAuthTestRedirectURI = "http://127.0.0.1:53127/newapi/callback"
)

type appAuthTestEnv struct {
	router  *gin.Engine
	user    *model.User
	session *model.UserSession
	jwt     string
	pat     string
}

type appAuthTestResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    struct {
		RedirectURL string `json:"redirect_url"`
		Key         string `json:"key"`
		Token       struct {
			ID    int    `json:"id"`
			Name  string `json:"name"`
			Group string `json:"group"`
		} `json:"token"`
		User struct {
			ID       int    `json:"id"`
			Username string `json:"username"`
		} `json:"user"`
	} `json:"data"`
}

func (env *appAuthTestEnv) post(t *testing.T, path string, credential string, body any) (int, appAuthTestResponse, string) {
	t.Helper()
	payload, err := common.Marshal(body)
	require.NoError(t, err)
	request := httptest.NewRequest("POST", path, strings.NewReader(string(payload)))
	request.Header.Set("Content-Type", "application/json")
	request.RemoteAddr = "192.0.2.30:4321"
	if credential != "" {
		request.Header.Set("Authorization", "Bearer "+credential)
	}
	recorder := httptest.NewRecorder()
	env.router.ServeHTTP(recorder, request)
	var response appAuthTestResponse
	if recorder.Code == 200 || recorder.Code == 401 {
		require.NoError(t, common.Unmarshal(recorder.Body.Bytes(), &response), recorder.Body.String())
	}
	return recorder.Code, response, recorder.Header().Get(common.RequestIdKey)
}

func appAuthRequest(overrides map[string]any) map[string]any {
	request := map[string]any{
		"client_name":           "Pier",
		"redirect_uri":          appAuthTestRedirectURI,
		"code_challenge":        pkceS256Challenge(appAuthTestVerifier),
		"code_challenge_method": "S256",
		"state":                 "state-123",
		"token":                 map[string]any{"name": "Pier · laptop", "unlimited_quota": true, "expired_time": -1, "group": "default"},
	}
	for key, value := range overrides {
		if value == nil {
			delete(request, key)
		} else {
			request[key] = value
		}
	}
	return request
}

// authorize approves the default request and returns the issued code.
func (env *appAuthTestEnv) authorize(t *testing.T) string {
	t.Helper()
	status, response, _ := env.post(t, "/api/app-auth/authorize", env.jwt, appAuthRequest(nil))
	require.Equal(t, 200, status)
	require.True(t, response.Success, response.Message)
	redirect, err := url.Parse(response.Data.RedirectURL)
	require.NoError(t, err)
	assert.Equal(t, "127.0.0.1:53127", redirect.Host)
	assert.Equal(t, "/newapi/callback", redirect.Path)
	assert.Equal(t, "state-123", redirect.Query().Get("state"))
	code := redirect.Query().Get("code")
	require.NotEmpty(t, code)
	return code
}

func (env *appAuthTestEnv) exchange(t *testing.T, code string, verifier string, redirectURI string) appAuthTestResponse {
	t.Helper()
	status, response, _ := env.post(t, "/api/app-auth/token", "", map[string]any{"code": code, "code_verifier": verifier, "redirect_uri": redirectURI})
	require.Equal(t, 200, status)
	return response
}

func setupAppAuthTest(t *testing.T, kind string, dsn string) *appAuthTestEnv {
	t.Helper()
	previousDB, previousLogDB := model.DB, model.LOG_DB
	previousMain, previousLog := common.MainDatabaseType(), common.LogDatabaseType()
	previousRedis, previousMaster, previousSecret, previousEnabled := common.RedisEnabled, common.IsMasterNode, common.SessionSecret, common.AppAuthorizationEnabled
	t.Cleanup(func() {
		model.DB, model.LOG_DB = previousDB, previousLogDB
		common.SetDatabaseTypes(previousMain, previousLog)
		common.RedisEnabled, common.IsMasterNode, common.SessionSecret, common.AppAuthorizationEnabled = previousRedis, previousMaster, previousSecret, previousEnabled
	})
	require.NoError(t, i18n.Init())
	gin.SetMode(gin.TestMode)
	common.RedisEnabled, common.IsMasterNode = false, true
	common.SessionSecret = "app-authorization-test-secret"
	common.AppAuthorizationEnabled = true
	t.Setenv("LOG_SQL_DSN", "")

	db, _ := newAuditTestDatabase(t, kind, dsn)
	model.DB = db
	dbType := map[string]common.DatabaseType{"sqlite": common.DatabaseTypeSQLite, "mysql": common.DatabaseTypeMySQL, "postgres": common.DatabaseTypePostgreSQL}[kind]
	common.SetDatabaseTypes(dbType, dbType)
	require.NoError(t, db.AutoMigrate(&model.User{}, &model.UserSession{}, &model.Token{}, &model.AuthFlow{}))
	require.NoError(t, model.InitLogDB())

	pat := "app-auth-pat-secret"
	user := &model.User{Username: "app-owner", DisplayName: "App Owner", Password: "placeholder", Role: common.RoleCommonUser, Status: common.UserStatusEnabled, Group: "default", AuthVersion: 1, AccessToken: &pat, AffCode: "app-owner"}
	require.NoError(t, db.Create(user).Error)
	session := &model.UserSession{SID: "app-auth-session", UserID: user.Id, Version: 1, UserAuthVersion: 1, Status: model.UserSessionStatusActive, RefreshHash: "placeholder", LoginMethod: "github", LastActiveAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Hour).Unix()}
	require.NoError(t, model.CreateUserSession(session))
	jwt, _, err := service.IssueAccessToken(service.AuthIdentity{UserID: user.Id, SessionID: session.SID, UserAuthVersion: 1, SessionVersion: 1})
	require.NoError(t, err)

	router := gin.New()
	router.Use(middleware.RequestId())
	router.POST("/api/app-auth/authorize", middleware.UserAuth(), middleware.TokenOperationAudit(), AuthorizeApp)
	router.POST("/api/app-auth/token", ExchangeAppAuthorization)
	return &appAuthTestEnv{router: router, user: user, session: session, jwt: jwt, pat: pat}
}

func TestAppAuthorizationDatabaseMatrix(t *testing.T) {
	for _, database := range []struct{ name, env string }{
		{"sqlite", ""},
		{"mysql", "AUDIT_MYSQL_DSN"},
		{"postgres", "AUDIT_POSTGRES_DSN"},
	} {
		t.Run(database.name, func(t *testing.T) {
			dsn := os.Getenv(database.env)
			if database.env != "" && dsn == "" {
				t.Skip(database.env + " is not configured")
			}
			verifyAppAuthorization(t, database.name, dsn)
		})
	}
}

func verifyAppAuthorization(t *testing.T, kind string, dsn string) {
	t.Run("code exchanges once for the approved token", func(t *testing.T) {
		env := setupAppAuthTest(t, kind, dsn)
		status, response, requestID := env.post(t, "/api/app-auth/authorize", env.jwt, appAuthRequest(nil))
		require.Equal(t, 200, status)
		require.True(t, response.Success, response.Message)
		redirect, err := url.Parse(response.Data.RedirectURL)
		require.NoError(t, err)
		code := redirect.Query().Get("code")
		require.NotEmpty(t, code)

		var token model.Token
		require.NoError(t, model.DB.Where("user_id = ?", env.user.Id).First(&token).Error)
		assert.Equal(t, "Pier · laptop", token.Name)

		exchanged := env.exchange(t, code, appAuthTestVerifier, appAuthTestRedirectURI)
		require.True(t, exchanged.Success, exchanged.Message)
		assert.Equal(t, token.GetFullKey(), exchanged.Data.Key)
		assert.Equal(t, token.Id, exchanged.Data.Token.ID)
		assert.Equal(t, "default", exchanged.Data.Token.Group)
		assert.Equal(t, "app-owner", exchanged.Data.User.Username)

		replay := env.exchange(t, code, appAuthTestVerifier, appAuthTestRedirectURI)
		assert.False(t, replay.Success)
		assert.Empty(t, replay.Data.Key)

		var events []model.AuditLog
		require.NoError(t, model.LOG_DB.Where("user_id = ?", env.user.Id).Order("id").Find(&events).Error)
		actions := map[string]bool{}
		for _, event := range events {
			if event.Success {
				actions[event.Action] = true
			}
		}
		assert.True(t, actions["token.app_authorize"], "authorization is audited")
		assert.True(t, actions["token.app_key_exchange"], "key exchange is audited")
		var authorizeEvent model.AuditLog
		require.NoError(t, model.LOG_DB.Where("request_id = ?", requestID).First(&authorizeEvent).Error)
		require.NotNil(t, authorizeEvent.Other.Op)
		params, err := common.Marshal(authorizeEvent.Other.Op.Params)
		require.NoError(t, err)
		assert.JSONEq(t, fmt.Sprintf(`{"id":%d,"name":"Pier · laptop","client_name":"Pier"}`, token.Id), string(params))
		encoded, err := common.Marshal(events)
		require.NoError(t, err)
		for _, secret := range []string{token.Key, code, appAuthTestVerifier, env.jwt} {
			assert.NotContains(t, string(encoded), secret)
		}
	})

	t.Run("a wrong verifier or redirect burns the code", func(t *testing.T) {
		env := setupAppAuthTest(t, kind, dsn)
		for _, attempt := range []struct{ verifier, redirect string }{
			{strings.Repeat("x", 43), appAuthTestRedirectURI},
			{appAuthTestVerifier, "http://127.0.0.1:53128/newapi/callback"},
		} {
			code := env.authorize(t)
			failed := env.exchange(t, code, attempt.verifier, attempt.redirect)
			assert.False(t, failed.Success)
			retried := env.exchange(t, code, appAuthTestVerifier, appAuthTestRedirectURI)
			assert.False(t, retried.Success, "the code must not survive a failed exchange")
			assert.Empty(t, retried.Data.Key)
		}
	})

	t.Run("expired codes, revoked sessions and disabled tokens are rejected", func(t *testing.T) {
		env := setupAppAuthTest(t, kind, dsn)
		expired := env.authorize(t)
		require.NoError(t, model.DB.Model(&model.AuthFlow{}).Where("purpose = ?", model.AuthFlowPurposeAppAuthorization).Update("expires_at", time.Now().Add(-time.Minute)).Error)
		assert.False(t, env.exchange(t, expired, appAuthTestVerifier, appAuthTestRedirectURI).Success)

		disabled := env.authorize(t)
		require.NoError(t, model.DB.Model(&model.Token{}).Where("user_id = ?", env.user.Id).Update("status", common.TokenStatusDisabled).Error)
		assert.False(t, env.exchange(t, disabled, appAuthTestVerifier, appAuthTestRedirectURI).Success)

		revoked := env.authorize(t)
		_, err := model.RevokeUserSession(env.user.Id, env.session.SID, "logout")
		require.NoError(t, err)
		response := env.exchange(t, revoked, appAuthTestVerifier, appAuthTestRedirectURI)
		assert.False(t, response.Success)
		assert.Empty(t, response.Data.Key)
	})

	t.Run("invalid requests create no token", func(t *testing.T) {
		env := setupAppAuthTest(t, kind, dsn)
		for index, overrides := range []map[string]any{
			{"redirect_uri": "https://attacker.example/callback"},
			{"redirect_uri": "http://localhost:53127/callback"},
			{"redirect_uri": "http://127.0.0.1/callback"},
			{"redirect_uri": "http://127.0.0.1:0/callback"},
			{"redirect_uri": "http://127.0.0.1:53127/callback?next=https://attacker.example"},
			{"redirect_uri": "http://127.0.0.1:53127/callback#fragment"},
			{"redirect_uri": "http://user@127.0.0.1:53127/callback"},
			{"redirect_uri": nil},
			{"code_challenge_method": "plain"},
			{"code_challenge": "too-short"},
			{"code_challenge": nil},
			{"client_name": "  "},
			{"client_name": "Pier\u202e"},
			{"client_name": strings.Repeat("a", appAuthorizationMaxClientName+1)},
			{"state": strings.Repeat("s", appAuthorizationMaxState+1)},
			{"token": map[string]any{"name": strings.Repeat("n", 51)}},
		} {
			status, response, _ := env.post(t, "/api/app-auth/authorize", env.jwt, appAuthRequest(overrides))
			assert.Equal(t, 200, status, "case %d", index)
			assert.False(t, response.Success, "case %d: %v", index, overrides)
			assert.Empty(t, response.Data.RedirectURL, "case %d", index)
		}
		status, response, _ := env.post(t, "/api/app-auth/authorize", env.pat, appAuthRequest(nil))
		assert.Equal(t, 401, status, "a personal access token cannot grant consent")
		assert.False(t, response.Success)

		common.AppAuthorizationEnabled = false
		_, response, _ = env.post(t, "/api/app-auth/authorize", env.jwt, appAuthRequest(nil))
		assert.False(t, response.Success, "disabled sites refuse authorization")

		var count int64
		require.NoError(t, model.DB.Model(&model.Token{}).Count(&count).Error)
		assert.Zero(t, count)
		require.NoError(t, model.DB.Model(&model.AuthFlow{}).Count(&count).Error)
		assert.Zero(t, count)
	})

	t.Run("exchange rejects malformed input and disabled sites", func(t *testing.T) {
		env := setupAppAuthTest(t, kind, dsn)
		code := env.authorize(t)
		for index, body := range []map[string]any{
			{"code": code, "code_verifier": "short", "redirect_uri": appAuthTestRedirectURI},
			{"code": code, "code_verifier": strings.Repeat("!", 43), "redirect_uri": appAuthTestRedirectURI},
			{"code": "", "code_verifier": appAuthTestVerifier, "redirect_uri": appAuthTestRedirectURI},
			{"code": "unknown-code", "code_verifier": appAuthTestVerifier, "redirect_uri": appAuthTestRedirectURI},
		} {
			_, response, _ := env.post(t, "/api/app-auth/token", "", body)
			assert.False(t, response.Success, fmt.Sprintf("case %d", index))
		}
		common.AppAuthorizationEnabled = false
		assert.False(t, env.exchange(t, code, appAuthTestVerifier, appAuthTestRedirectURI).Success)
		common.AppAuthorizationEnabled = true
		// Malformed attempts never reached a live grant, so the code still works once.
		exchanged := env.exchange(t, code, appAuthTestVerifier, appAuthTestRedirectURI)
		assert.True(t, exchanged.Success, exchanged.Message)
	})
}
