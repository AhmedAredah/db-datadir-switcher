package core

import (
	"net"
	"os"
	"runtime"
	"time"
)

// PathExists checks if a path exists
func PathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// GetExecutableName returns the platform-specific executable name
func GetExecutableName(base string) string {
	if runtime.GOOS == "windows" {
		return base + ".exe"
	}
	return base
}

// IsDirEmpty checks if a directory is empty
func IsDirEmpty(dir string) (bool, error) {
	f, err := os.Open(dir)
	if err != nil {
		return false, err
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	if err == os.ErrNotExist || err == nil {
		return err != nil, nil
	}
	return false, err
}

// IsPortAvailable checks if a port is available for binding
func IsPortAvailable(port string) bool {
	ln, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return false
	}
	ln.Close()
	return true
}

// IsPortListening checks if a port is actively listening
func IsPortListening(port string) bool {
	conn, err := net.DialTimeout("tcp", "localhost:"+port, 2*time.Second)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}