package redirect

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/containous/traefik/pkg/config"
	"github.com/containous/traefik/pkg/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRedirectRegexHandler(t *testing.T) {
	testCases := []struct {
		desc           string
		config         config.RedirectRegex
		method         string
		url            string
		secured        bool
		expectedURL    string
		expectedStatus int
		errorExpected  bool
	}{
		{
			desc: "simple redirection",
			config: config.RedirectRegex{
				Regex:       `^(?:http?:\/\/)(foo)(\.com)(:\d+)(.*)$`,
				Replacement: "https://${1}bar$2:443$4",
			},
			url:            "http://foo.com:80",
			expectedURL:    "https://foobar.com:443",
			expectedStatus: http.StatusFound,
		},
		{
			desc: "use request header",
			config: config.RedirectRegex{
				Regex:       `^(?:http?:\/\/)(foo)(\.com)(:\d+)(.*)$`,
				Replacement: `https://${1}{{ .Request.Header.Get "X-Foo" }}$2:443$4`,
			},
			url:            "http://foo.com:80",
			expectedURL:    "https://foobar.com:443",
			expectedStatus: http.StatusFound,
		},
		{
			desc: "URL doesn't match regex",
			config: config.RedirectRegex{
				Regex:       `^(?:http?:\/\/)(foo)(\.com)(:\d+)(.*)$`,
				Replacement: "https://${1}bar$2:443$4",
			},
			url:            "http://bar.com:80",
			expectedStatus: http.StatusOK,
		},
		{
			desc: "invalid rewritten URL",
			config: config.RedirectRegex{
				Regex:       `^(.*)$`,
				Replacement: "http://192.168.0.%31/",
			},
			url:            "http://foo.com:80",
			expectedStatus: http.StatusBadGateway,
		},
		{
			desc: "invalid regex",
			config: config.RedirectRegex{
				Regex:       `^(.*`,
				Replacement: "$1",
			},
			url:           "http://foo.com:80",
			errorExpected: true,
		},
		{
			desc: "HTTP to HTTPS permanent",
			config: config.RedirectRegex{
				Regex:       `^http://`,
				Replacement: "https://$1",
				Permanent:   true,
			},
			url:            "http://foo",
			expectedURL:    "https://foo",
			expectedStatus: http.StatusMovedPermanently,
		},
		{
			desc: "HTTPS to HTTP permanent",
			config: config.RedirectRegex{
				Regex:       `https://foo`,
				Replacement: "http://foo",
				Permanent:   true,
			},
			secured:        true,
			url:            "https://foo",
			expectedURL:    "http://foo",
			expectedStatus: http.StatusMovedPermanently,
		},
		{
			desc: "HTTP to HTTPS",
			config: config.RedirectRegex{
				Regex:       `http://foo:80`,
				Replacement: "https://foo:443",
			},
			url:            "http://foo:80",
			expectedURL:    "https://foo:443",
			expectedStatus: http.StatusFound,
		},
		{
			desc: "HTTPS to HTTP",
			config: config.RedirectRegex{
				Regex:       `https://foo:443`,
				Replacement: "http://foo:80",
			},
			secured:        true,
			url:            "https://foo:443",
			expectedURL:    "http://foo:80",
			expectedStatus: http.StatusFound,
		},
		{
			desc: "HTTP to HTTP",
			config: config.RedirectRegex{
				Regex:       `http://foo:80`,
				Replacement: "http://foo:88",
			},
			url:            "http://foo:80",
			expectedURL:    "http://foo:88",
			expectedStatus: http.StatusFound,
		},
		{
			desc: "HTTP to HTTPS POST",
			config: config.RedirectRegex{
				Regex:       `^http://`,
				Replacement: "https://$1",
			},
			url:            "http://foo",
			method:         http.MethodPost,
			expectedURL:    "https://foo",
			expectedStatus: http.StatusTemporaryRedirect,
		},
		{
			desc: "HTTP to HTTPS POST permanent",
			config: config.RedirectRegex{
				Regex:       `^http://`,
				Replacement: "https://$1",
				Permanent:   true,
			},
			url:            "http://foo",
			method:         http.MethodPost,
			expectedURL:    "https://foo",
			expectedStatus: http.StatusPermanentRedirect,
		},
	}

	for _, test := range testCases {
		test := test
		t.Run(test.desc, func(t *testing.T) {
			t.Parallel()

			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
			handler, err := NewRedirectRegex(context.Background(), next, test.config, "traefikTest")

			if test.errorExpected {
				require.Error(t, err)
				require.Nil(t, handler)
			} else {
				require.NoError(t, err)
				require.NotNil(t, handler)

				recorder := httptest.NewRecorder()

				method := http.MethodGet
				if test.method != "" {
					method = test.method
				}
				r := testhelpers.MustNewRequest(method, test.url, nil)
				if test.secured {
					r.TLS = &tls.ConnectionState{}
				}
				r.Header.Set("X-Foo", "bar")
				handler.ServeHTTP(recorder, r)

				assert.Equal(t, test.expectedStatus, recorder.Code)
				if test.expectedStatus == http.StatusMovedPermanently ||
					test.expectedStatus == http.StatusFound ||
					test.expectedStatus == http.StatusTemporaryRedirect ||
					test.expectedStatus == http.StatusPermanentRedirect {

					location, err := recorder.Result().Location()
					require.NoError(t, err)

					assert.Equal(t, test.expectedURL, location.String())
				} else {
					location, err := recorder.Result().Location()
					require.Errorf(t, err, "Location %v", location)
				}
			}
		})
	}
}
