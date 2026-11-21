//go:build unix

package main

import "syscall"

// noFollowFlag makes os.OpenFile refuse a symlink at the staged path, closing
// the window between writeFresh's Lstat and its create (D, file staging).
const noFollowFlag = syscall.O_NOFOLLOW
