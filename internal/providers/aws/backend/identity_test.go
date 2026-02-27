package aws

import (
	"testing"
)

func TestCallerIdentity_parseARN(t *testing.T) {
	tests := []struct {
		name          string
		arn           string
		wantErr       bool
		wantRole      string
		wantSession   string
		wantPrincipal PrincipalType
	}{
		{
			name:          "valid assumed role ARN",
			arn:           "arn:aws:sts::123456789012:assumed-role/MyRole/MySession",
			wantErr:       false,
			wantRole:      "MyRole",
			wantSession:   "MySession",
			wantPrincipal: PrincipalAssumedRole,
		},
		{
			name:    "invalid arn format",
			arn:     "not-an-arn",
			wantErr: true,
		},
		{
			name:    "unsupported resource type",
			arn:     "arn:aws:iam::123456789012:user/Bob",
			wantErr: true,
		},
		{
			name:    "assumed role missing parts",
			arn:     "arn:aws:sts::123456789012:assumed-role/MyRole",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := &CallerIdentity{
				ARN: tt.arn,
			}

			err := id.parseARN()

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if id.RoleName != tt.wantRole {
				t.Errorf("RoleName = %s, want %s", id.RoleName, tt.wantRole)
			}

			if id.SessionName != tt.wantSession {
				t.Errorf("SessionName = %s, want %s", id.SessionName, tt.wantSession)
			}

			if id.PrincipalType != tt.wantPrincipal {
				t.Errorf("PrincipalType = %v, want %v", id.PrincipalType, tt.wantPrincipal)
			}
		})
	}
}

func TestCallerIdentity_WhoAmI(t *testing.T) {
	tests := []struct {
		name     string
		identity CallerIdentity
		expected string
	}{
		{
			name: "valid assumed role identity",
			identity: CallerIdentity{
				PrincipalType: PrincipalAssumedRole,
				AccountID:     "123456789012",
				Region:        "us-east-1",
				RoleName:      "MyRole",
			},
			expected: "MyRole (123456789012) / us-east-1",
		},
		{
			name: "missing account id",
			identity: CallerIdentity{
				PrincipalType: PrincipalAssumedRole,
				Region:        "us-east-1",
				RoleName:      "MyRole",
			},
			expected: "Account Unknown",
		},
		{
			name: "wrong principal type",
			identity: CallerIdentity{
				PrincipalType: "SomeOtherType",
				AccountID:     "123456789012",
				Region:        "us-east-1",
				RoleName:      "MyRole",
			},
			expected: "Account Unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.identity.WhoAmI()
			if result != tt.expected {
				t.Errorf("WhoAmI() = %s, want %s", result, tt.expected)
			}
		})
	}
}
