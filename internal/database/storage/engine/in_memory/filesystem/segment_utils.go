package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// CreateFile opens a fresh WAL segment file inside directory. The file name
// is maskName + unix-nanoseconds + ".log"; O_EXCL guarantees that two
// concurrent callers will not collide on the same name.
func CreateFile(directory, maskName string) (*os.File, error) {
	fileName := fmt.Sprintf("%s%d.log", maskName, time.Now().UnixNano())
	flags := os.O_CREATE | os.O_EXCL | os.O_WRONLY | os.O_APPEND
	filePath := filepath.Join(directory, fileName)

	file, err := os.OpenFile(filePath, flags, 0o600)
	if err != nil {
		return nil, err
	}

	return file, nil
}

// WriteFile appends data to file and fsyncs it to disk. Returns the number
// of bytes written and any error from either Write or Sync.
func WriteFile(file *os.File, data []byte) (int, error) {
	writtenBytes, err := file.Write(data)
	if err != nil {
		return 0, err
	}
	err = file.Sync()
	if err != nil {
		return 0, err
	}

	return writtenBytes, nil
}

// ReadFile loads the entire contents of filePath into memory.
func ReadFile(filePath string) ([]byte, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// GetFileList returns the sorted list of regular-file paths inside
// directory. Directories are skipped. The list is sorted lexicographically
// which, combined with the nanosecond-based file names produced by
// [CreateFile], yields chronological order.
func GetFileList(directory string) ([]string, error) {
	files, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}

	filesNames := make([]string, 0)
	for _, file := range files {

		if file.IsDir() {
			continue
		}

		filePath := filepath.Join(directory, file.Name())
		filesNames = append(filesNames, filePath)
	}
	sort.Strings(filesNames)
	return filesNames, nil
}
