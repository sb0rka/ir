package mockdata

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/sb0rka/ir/apps/gateway/internal/domain"
)

var KnownArtifactNames = []string{
	"malicious_office_document.docx",
	"microsoftonedrive.exe",
	"veeam_1272",
	"pilot.ps1",
	"shell.php",
	"transfer.php",
}

func Artifact(name string) domain.Artifact {
	name = strings.ToLower(strings.TrimSpace(name))
	md5sum := md5.Sum([]byte(name))
	sha1sum := sha1.Sum([]byte(name))
	sha256sum := sha256.Sum256([]byte(name))
	return domain.Artifact{
		ID:   domain.StableID("artifact", name),
		Name: name,
		MIME: mimeFor(name),
		Size: sizeFor(name),
		Hashes: domain.Hashes{
			MD5:    hex.EncodeToString(md5sum[:]),
			SHA1:   hex.EncodeToString(sha1sum[:]),
			SHA256: hex.EncodeToString(sha256sum[:]),
		},
	}
}

func FindArtifact(name string, hashes domain.Hashes) (domain.Artifact, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, known := range KnownArtifactNames {
		artifact := Artifact(known)
		if name == known || hashes.SHA256 != "" && strings.EqualFold(hashes.SHA256, artifact.Hashes.SHA256) ||
			hashes.SHA1 != "" && strings.EqualFold(hashes.SHA1, artifact.Hashes.SHA1) ||
			hashes.MD5 != "" && strings.EqualFold(hashes.MD5, artifact.Hashes.MD5) {
			return artifact, true
		}
	}
	return domain.Artifact{}, false
}

func mimeFor(name string) string {
	switch {
	case strings.HasSuffix(name, ".docx"):
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case strings.HasSuffix(name, ".exe"):
		return "application/x-dosexec"
	case strings.HasSuffix(name, ".ps1"):
		return "text/x-powershell"
	case strings.HasSuffix(name, ".php"):
		return "text/x-php"
	default:
		return "application/octet-stream"
	}
}

func sizeFor(name string) int64 {
	switch name {
	case "malicious_office_document.docx":
		return 48128
	case "microsoftonedrive.exe":
		return 317440
	case "veeam_1272":
		return 923136
	case "pilot.ps1":
		return 6144
	case "shell.php":
		return 2048
	case "transfer.php":
		return 1536
	default:
		return 0
	}
}
