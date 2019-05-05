// +build !windows

package scanner

import (
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
