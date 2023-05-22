/*
Copyright 2023 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package filesystem

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"k8s.io/klog/v2"
	util "sigs.k8s.io/azuredisk-csi-driver/pkg/azureutils"
)

var invalidPathCharsRegexWindows = regexp.MustCompile(`["/\:\?\*|]`)
var absPathRegexWindows = regexp.MustCompile(`^[a-zA-Z]:\\`)

func containsInvalidCharactersWindows(path string) bool {
	if isAbsWindows(path) {
		path = path[3:]
	}
	if invalidPathCharsRegexWindows.MatchString(path) {
		return true
	}
	if strings.Contains(path, `..`) {
		return true
	}
	return false
}

func isUNCPathWindows(path string) bool {
	// check for UNC/pipe prefixes like "\\"
	if len(path) < 2 {
		return false
	}
	if path[0] == '\\' && path[1] == '\\' {
		return true
	}
	return false
}

func isAbsWindows(path string) bool {
	// for Windows check for C:\\.. prefix only
	// UNC prefixes of the form \\ are not considered
	return absPathRegexWindows.MatchString(path)
}

func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// PathExists checks if the given path exists on the host.
func PathExists(path string) (bool, error) {
	if err := ValidatePathWindows(path); err != nil {
		klog.Errorf("failed validatePathWindows %v", err)
		return false, err
	}
	return pathExists(path)
}

func PathValid(ctx context.Context, path string) (bool, error) {
	cmd := `Test-Path $Env:remotepath`
	cmdEnv := fmt.Sprintf("remotepath=%s", path)
	output, err := util.RunPowershellCmd(cmd, cmdEnv)
	if err != nil {
		return false, fmt.Errorf("returned output: %s, error: %v", string(output), err)
	}

	return strings.HasPrefix(strings.ToLower(string(output)), "true"), nil
}

func ValidatePathWindows(path string) error {
	pathlen := len(path)
	if pathlen > util.MaxPathLengthWindows {
		return fmt.Errorf("path length %d exceeds maximum characters: %d", pathlen, util.MaxPathLengthWindows)
	}

	if pathlen > 0 && (path[0] == '\\') {
		return fmt.Errorf("invalid character \\ at beginning of path: %s", path)
	}

	if isUNCPathWindows(path) {
		return fmt.Errorf("unsupported UNC path prefix: %s", path)
	}

	if containsInvalidCharactersWindows(path) {
		return fmt.Errorf("path contains invalid characters: %s", path)
	}

	if !isAbsWindows(path) {
		return fmt.Errorf("not an absolute Windows path: %s", path)
	}

	return nil
}

func Mkdir(path string) error {
	if err := ValidatePathWindows(path); err != nil {
		return err
	}
	return os.MkdirAll(path, 0755)
}

func Rmdir(path string, force bool) error {
	if err := ValidatePathWindows(path); err != nil {
		return err
	}

	if force {
		return os.RemoveAll(path)
	}
	return os.Remove(path)
}

func LinkPath(sourcePath, targetPath string) error {
	return CreateSymlink(sourcePath, targetPath)
}

func CreateSymlink(sourcePath, targetPath string) error {
	if err := ValidatePathWindows(targetPath); err != nil {
		return err
	}
	if err := ValidatePathWindows(sourcePath); err != nil {
		return err
	}
	return os.Symlink(sourcePath, targetPath)
}

func IsMountPoint(path string) (bool, error) {
	return IsSymlink(path)
}

func IsSymlink(path string) (bool, error) {
	return isSymlink(path)
}

// IsSymlink - returns true if tgt is a mount point.
// A path is considered a mount point if:
// - directory exists and
// - it is a soft link and
// - the target path of the link exists.
// If tgt path does not exist, it returns an error
// if tgt path exists, but the source path tgt points to does not exist, it returns false without error.
func isSymlink(tgt string) (bool, error) {
	// This code is similar to k8s.io/kubernetes/pkg/util/mount except the pathExists usage.
	stat, err := os.Lstat(tgt)
	if err != nil {
		return false, err
	}

	// If its a link and it points to an existing file then its a mount point.
	if stat.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(tgt)
		if err != nil {
			return false, fmt.Errorf("readlink error: %v", err)
		}
		exists, err := pathExists(target)
		if err != nil {
			return false, err
		}
		return exists, nil
	}

	return false, nil
}
