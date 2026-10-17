//go:build windows

package cli

import (
	"os"
	"path/filepath"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

func setWindowsTestDACL(t *testing.T, path, dacl string) {
	t.Helper()
	sd, err := windows.SecurityDescriptorFromString(dacl)
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
}

func windowsTestSID(t *testing.T) *windows.SID {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	return user.User.Sid
}

func assertWindowsPrivate(t *testing.T, path string) {
	t.Helper()
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	acl, _, err := sd.DACL()
	if err != nil || acl == nil {
		t.Fatalf("private file has no DACL: %v", err)
	}
	current := windowsTestSID(t)
	for i := uint32(0); i < uint32(acl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(acl, i, &ace); err != nil {
			t.Fatal(err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if !sid.Equals(current) && !sid.IsWellKnown(windows.WinLocalSystemSid) && !sid.IsWellKnown(windows.WinBuiltinAdministratorsSid) {
			t.Fatal("private file permits another principal")
		}
	}
}

func TestWindowsConfigRejectsPublicACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("broker_url=http://127.0.0.1:8787\npairing_token=fixture-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	setWindowsTestDACL(t, path, "D:P(A;;FA;;;"+windowsTestSID(t).String()+")(A;;FR;;;WD)")
	if _, err := LoadSettings(path); err == nil {
		t.Fatal("accepted config readable by Everyone")
	}
}

func TestWindowsConfigAcceptsUserSystemAndAdministrators(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config")
	if err := os.WriteFile(path, []byte("broker_url=http://127.0.0.1:8787\npairing_token=fixture-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	setWindowsTestDACL(t, path, "D:P(A;;FA;;;"+windowsTestSID(t).String()+")(A;;FA;;;SY)(A;;FA;;;BA)")
	if _, err := LoadSettings(path); err != nil {
		t.Fatalf("rejected private default Windows principals: %v", err)
	}
}

func TestWindowsWriteSecretStopsBeforeFailedACLProtection(t *testing.T) {
	dir := t.TempDir()
	setWindowsTestDACL(t, dir, "D:P(D;OIIO;WD;;;OW)(A;OICI;FA;;;"+windowsTestSID(t).String()+")")
	path := filepath.Join(dir, "secret")
	if err := WriteSecret(path, []byte("fixture-secret")); err == nil {
		t.Fatal("reported successful write despite denied ACL protection")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed protected write left files: count=%d err=%v", len(entries), err)
	}
}

func TestWindowsPrivateFileRemovesExplicitPublicGrant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	setWindowsTestDACL(t, path, "D:P(A;;FA;;;"+windowsTestSID(t).String()+")(A;;FR;;;WD)")
	hardenPrivateFile(path)
	assertWindowsPrivate(t, path)
}

func TestWindowsSSHIdentityRejectsInheritedPublicAccess(t *testing.T) {
	dir := t.TempDir()
	setWindowsTestDACL(t, dir, "D:P(A;OICI;FA;;;"+windowsTestSID(t).String()+")(A;OICI;FR;;;WD)")
	path := filepath.Join(dir, "identity")
	if err := WriteProtectedFile(path, []byte("fixture-private-key"), 0o600); err != nil {
		t.Fatal(err)
	}
	assertWindowsPrivate(t, path)
}
