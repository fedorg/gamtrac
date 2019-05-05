// +build windows

package scanner

import "golang.org/x/sys/windows"
import acl "github.com/hectane/go-acl/api"

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
