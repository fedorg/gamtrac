// +build windows

package scanner

import (
	"fmt"
	"os"
	"os/exec"

	acl "github.com/hectane/go-acl/api"
	"golang.org/x/sys/windows"
)

func GetFileOwnerUID(filename string) (*string, error) {
	var (
		owner   *windows.SID
		secDesc windows.Handle
	)
	err := acl.GetNamedSecurityInfo(
		filename,
		acl.SE_FILE_OBJECT,
		acl.OWNER_SECURITY_INFORMATION,
		&owner,
		nil,
		nil,
		nil,
		&secDesc,
	)
	if err != nil {
		return nil, err
	}
	defer windows.LocalFree(secDesc)
	sidString, err := owner.String()
	if err != nil {
		return nil, err
	}
	// account, domain, accType, err := owner.LookupAccount("")
	// fmt.Println(account, domain, accType, sidString, err)
	return &sidString, nil
}

// Remove a drive
func UnmountShare(local string) ([]byte, error) {
	return exec.Command("net", "use", local, "/delete").CombinedOutput()
}

func MountShare(share string, domain string, user string, pass string) (*string, error) {
	output, err := exec.Command("net", "use", share, "/delete", "/y").CombinedOutput()
	output, err = exec.Command("net", "use", share, fmt.Sprintf(`/user:%s\%s`, domain, user), pass, "/y").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", string(output))
		return nil, err
	}
	tmpdir := share
	return &tmpdir, nil
}
