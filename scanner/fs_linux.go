// +build !windows

package scanner

import (
	"io/ioutil"
	"fmt"
	"os"
	"os/user"
	"syscall"
)

func GetFileOwnerUID(filename string) (*string, error) {
	fi, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}
	stat, err := fi.Sys().(*syscall.Stat_t)
	if err != nil {
		return nil, err
	}
	fileUser, err := user.LookupId(fmt.Sprintf("%v", stat.Uid))
	if err != nil {
		return nil, err
	}
	return &fileUser.Name
}

func UnmountShare(local string) ([]byte, error) {
	ret, err := exec.Command("umount", local).CombinedOutput()
	if err != nil {
		return ret, err
	}
	ret, err = exec.Command("rmdir", local) // should be empty
	return ret, err
}

func MountShare(share string, domain string, user string, pass string) (*string, error) {
	local, err := ioutil.TempDir("/tmp", "gamtrac_")
	if err != nil {
		return nil, err
	}
	mntopt := fmt.Sprintf(`user=%s\%s,password=%s,vers=3.0`, user, domain, pass)
	output, err := exec.Command("mount", "-t", "cifs", "-o", mntopt, share, local).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", string(output))
		return nil, err
	}
	return &local, nil
}
