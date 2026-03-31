/*
	Copyright NetFoundry Inc.

	Licensed under the Apache License, Version 2.0 (the "License");
	you may not use this file except in compliance with the License.
	You may obtain a copy of the License at

	https://www.apache.org/licenses/LICENSE-2.0

	Unless required by applicable law or agreed to in writing, software
	distributed under the License is distributed on an "AS IS" BASIS,
	WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	See the License for the specific language governing permissions and
	limitations under the License.
*/

package spiffehlp

import (
	"net/url"
	"testing"
)

func TestVerifySpiffeId(t *testing.T) {
	tests := []struct {
		name       string
		uri        string
		identityId string
		sessionId  string
		want       SpiffeMatch
	}{
		{
			name:       "6-segment api session cert",
			uri:        "spiffe://trust-domain/identity/id1/apiSession/session1/apiSessionCertificate/cert1",
			identityId: "id1",
			sessionId:  "session1",
			want:       SpiffeMatchApiSession,
		},
		{
			name:       "4-segment api session (legacy fallback)",
			uri:        "spiffe://trust-domain/identity/id1/apiSession/session1",
			identityId: "id1",
			sessionId:  "session1",
			want:       SpiffeMatchApiSession,
		},
		{
			name:       "2-segment identity only",
			uri:        "spiffe://trust-domain/identity/id1",
			identityId: "id1",
			sessionId:  "session1",
			want:       SpiffeMatchIdentity,
		},
		{
			name:       "wrong identity id",
			uri:        "spiffe://trust-domain/identity/wrong/apiSession/session1/apiSessionCertificate/cert1",
			identityId: "id1",
			sessionId:  "session1",
			want:       SpiffeMatchNone,
		},
		{
			name:       "wrong api session id",
			uri:        "spiffe://trust-domain/identity/id1/apiSession/wrong/apiSessionCertificate/cert1",
			identityId: "id1",
			sessionId:  "session1",
			want:       SpiffeMatchNone,
		},
		{
			name:       "identity matches but session does not (4-segment)",
			uri:        "spiffe://trust-domain/identity/id1/apiSession/wrong",
			identityId: "id1",
			sessionId:  "session1",
			want:       SpiffeMatchNone,
		},
		{
			name:       "wrong identity on 2-segment path",
			uri:        "spiffe://trust-domain/identity/wrong",
			identityId: "id1",
			sessionId:  "session1",
			want:       SpiffeMatchNone,
		},
		{
			name:       "wrong scheme",
			uri:        "https://trust-domain/identity/id1/apiSession/session1",
			identityId: "id1",
			sessionId:  "session1",
			want:       SpiffeMatchNone,
		},
		{
			name:       "wrong first segment",
			uri:        "spiffe://trust-domain/notidentity/id1/apiSession/session1",
			identityId: "id1",
			sessionId:  "session1",
			want:       SpiffeMatchNone,
		},
		{
			name:       "wrong third segment",
			uri:        "spiffe://trust-domain/identity/id1/notApiSession/session1",
			identityId: "id1",
			sessionId:  "session1",
			want:       SpiffeMatchNone,
		},
		{
			name:       "wrong fifth segment",
			uri:        "spiffe://trust-domain/identity/id1/apiSession/session1/notApiSessionCertificate/cert1",
			identityId: "id1",
			sessionId:  "session1",
			want:       SpiffeMatchNone,
		},
		{
			name:       "too few segments",
			uri:        "spiffe://trust-domain/identity",
			identityId: "id1",
			sessionId:  "session1",
			want:       SpiffeMatchNone,
		},
		{
			name:       "3 segments (invalid)",
			uri:        "spiffe://trust-domain/identity/id1/apiSession",
			identityId: "id1",
			sessionId:  "session1",
			want:       SpiffeMatchNone,
		},
		{
			name:       "5 segments (invalid)",
			uri:        "spiffe://trust-domain/identity/id1/apiSession/session1/extra",
			identityId: "id1",
			sessionId:  "session1",
			want:       SpiffeMatchNone,
		},
		{
			name:       "7 segments (too many)",
			uri:        "spiffe://trust-domain/identity/id1/apiSession/session1/apiSessionCertificate/cert1/extra",
			identityId: "id1",
			sessionId:  "session1",
			want:       SpiffeMatchNone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.uri)
			if err != nil {
				t.Fatalf("failed to parse URI: %v", err)
			}
			got := VerifySpiffeId(u, tt.identityId, tt.sessionId)
			if got != tt.want {
				t.Errorf("VerifySpiffeId(%q, %q, %q) = %v, want %v", tt.uri, tt.identityId, tt.sessionId, got, tt.want)
			}
		})
	}
}
