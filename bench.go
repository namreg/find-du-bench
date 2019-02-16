package bench

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type UsageInfo struct {
	Bytes, Inodes uint64
}

func DiskUsageNew(dir string) (UsageInfo, error) {
	var usage UsageInfo

	if dir == "" {
		return usage, fmt.Errorf("invalid directory")
	}

	rootInfo, err := os.Stat(dir)
	if err != nil {
		return usage, fmt.Errorf("could not stat %q to get inode usage: %v", dir, err)
	}

	rootStat, ok := rootInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return usage, fmt.Errorf("unsuported fileinfo for getting inode usage of %q", dir)
	}

	rootDevId := rootStat.Dev

	// dedupedInode stores inodes that could be duplicates (nlink > 1)
	dedupedInodes := make(map[uint64]struct{})

	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if os.IsNotExist(err) {
			// expected if files appear/vanish
			return nil
		}
		if err != nil {
			return fmt.Errorf("unable to count inodes for part of dir %s: %s", dir, err)
		}

		// according to the docs, Sys can be nil
		if info.Sys() == nil {
			return fmt.Errorf("fileinfo Sys is nil")
		}

		s, ok := info.Sys().(*syscall.Stat_t)
		if !ok {
			return fmt.Errorf("unsupported fileinfo; could not convert to stat_t")
		}

		if s.Dev != rootDevId {
			// don't descend into directories on other devices
			return filepath.SkipDir
		}
		if s.Nlink > 1 {
			if _, ok := dedupedInodes[s.Ino]; !ok {
				// Dedupe things that could be hardlinks
				dedupedInodes[s.Ino] = struct{}{}

				usage.Bytes += uint64(s.Blocks) * uint64(512)
				usage.Inodes++
			}
		} else {
			usage.Bytes += uint64(s.Blocks) * uint64(512)
			usage.Inodes++
		}
		return nil
	})

	return usage, nil
}

func GetDirDiskUsage(dir string, timeout time.Duration) (uint64, error) {
	if dir == "" {
		return 0, fmt.Errorf("invalid directory")
	}
	cmd := exec.Command("ionice", "-c3", "nice", "-n", "19", "du", "-s", dir)
	stdoutp, err := cmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to setup stdout for cmd %v - %v", cmd.Args, err)
	}

	stderrp, err := cmd.StderrPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to setup stderr for cmd %v - %v", cmd.Args, err)
	}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to exec du - %v", err)
	}

	timer := time.AfterFunc(timeout, func() {
		log.Printf("Killing cmd %v due to timeout(%s)", cmd.Args, timeout.String())
		cmd.Process.Kill()
	})

	stdoutb, souterr := ioutil.ReadAll(stdoutp)
	if souterr != nil {
		log.Fatal(souterr.Error())
	}
	stderrb, _ := ioutil.ReadAll(stderrp)
	err = cmd.Wait()
	timer.Stop()
	if err != nil {
		return 0, fmt.Errorf("du command failed on %s with output stdout: %s, stderr: %s - %v", dir, string(stdoutb), string(stderrb), err)
	}

	stdout := string(stdoutb)
	usageInKb, err := strconv.ParseUint(strings.Fields(stdout)[0], 10, 64)

	if err != nil {
		return 0, fmt.Errorf("cannot parse 'du' output %s - %s", stdout, err)
	}

	return usageInKb * 1024, nil
}

func GetDirInodeUsage(dir string, timeout time.Duration) (uint64, error) {
	if dir == "" {
		return 0, fmt.Errorf("invalid directory")
	}
	var counter byteCounter
	var stderr bytes.Buffer

	findCmd := exec.Command("ionice", "-c3", "nice", "-n", "19", "find", dir, "-xdev", "-printf", ".")
	findCmd.Stdout, findCmd.Stderr = &counter, &stderr
	if err := findCmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to exec cmd %v - %v; stderr: %v", findCmd.Args, err, stderr.String())
	}

	timer := time.AfterFunc(timeout, func() {
		log.Printf("Killing cmd %v due to timeout(%s)", findCmd.Args, timeout.String())
		findCmd.Process.Kill()
	})

	err := findCmd.Wait()
	timer.Stop()
	if err != nil {
		return 0, fmt.Errorf("cmd %v failed. stderr: %s; err: %v", findCmd.Args, stderr.String(), err)
	}

	return counter.bytesWritten, nil
}

// Simple io.Writer implementation that counts how many bytes were written.
type byteCounter struct{ bytesWritten uint64 }

func (b *byteCounter) Write(p []byte) (int, error) {
	b.bytesWritten += uint64(len(p))
	return len(p), nil
}
