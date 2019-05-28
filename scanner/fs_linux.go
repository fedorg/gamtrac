// +build !windows

package scanner

import (
	"io/ioutil"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"syscall"
)

func GetFileOwnerUID(filename string) (*string, error) {
	fi, err := os.Stat(filename)
	if err != nil {
		return nil, err
	}
	stat, is_err := fi.Sys().(*syscall.Stat_t)
	if is_err {
		return nil, fmt.Errorf("syscall to get file owner failed")
	}
	fileUser, err := user.LookupId(fmt.Sprintf("%v", stat.Uid))
	if err != nil {
		return nil, err
	}
	return &fileUser.Name, nil
}

func UnmountShare(local string) ([]byte, error) {
	ret, err := exec.Command("umount", local).CombinedOutput()
	if err != nil {
		return ret, err
	}
	ret, err = exec.Command("rmdir", local).CombinedOutput() // should be empty
	return ret, err
}

func MountShare(share string, domain string, user string, pass string) (*string, error) {
	local, err := ioutil.TempDir("/tmp", "gamtrac_")
	if err != nil {
		return nil, err
	}
	mntopt := fmt.Sprintf(`user=%s,password=%s`, user, pass)
	args := []string{"-t", "cifs", "-o", mntopt, share, local}
	output, err := exec.Command("mount", args...).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v", string(output))
		return nil, err
	}
	return &local, nil
}
