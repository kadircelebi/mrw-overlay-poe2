package filter

import "golang.org/x/sys/windows"

// knownDocumentsDir asks the shell for the Documents folder, wherever the
// user or OneDrive has moved it.
func knownDocumentsDir() string {
	p, err := windows.KnownFolderPath(windows.FOLDERID_Documents, 0)
	if err != nil {
		return ""
	}
	return p
}
