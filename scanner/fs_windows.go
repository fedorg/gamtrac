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

// Helper function to check for a used drive and enumerate through available drives
func findUnusedDrive() (*string, error) {
	alpha := []string{"Q", "R", "S", "T", "U", "V", "W", "X", "Y", "Z",
		"A", "B", "C", "D", "E", "F", "G", "H", "I", "J", "K", "L", "M",
		"N", "O", "P"}
	for _, letter := range alpha {
		if _, err := os.Stat(letter + `:\`); os.IsNotExist(err) {
			return &letter, nil
		}
	}
	return nil, fmt.Errorf("could not find an empty drive letter to mount things into")
}

// Remove a drive
func UnmountShare(local string) ([]byte, error) {
	return exec.Command("net", "use", local, "/delete").CombinedOutput()
}

func MountShare(share string, domain string, user string, pass string) (*string, error) {
	letter, err := findUnusedDrive()
	if err != nil {
		return nil, err
	}
	tmpdir := *letter + `:`
	fmt.Printf("%v", []string{"net", "use", tmpdir, share, fmt.Sprintf(`/user:%s\%s`, domain, user), pass, "/P:YES"})
	output, err := exec.Command("net", "use", tmpdir, share, fmt.Sprintf(`/user:%s\%s`, domain, user), pass, "/P:YES").CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", string(output))
		return nil, err
	}
	return &tmpdir, nil
}
