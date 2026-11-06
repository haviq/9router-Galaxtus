package dashboard

import (
	json "encoding/json/v2"
	"net/http"

	"9router/proxy/internal/coprocessor"
)

type BinaryLintRequest struct {
	Code     string   `json:"code"`
	BadBytes []string `json:"bad_bytes"`
	TargetOS string   `json:"target_os"`
}

type BinaryLintResponse struct {
	Shellcode *coprocessor.ShellcodeScanResult `json:"shellcode,omitempty"`
	Assembly  []coprocessor.LintIssue          `json:"assembly,omitempty"`
}

func (h *DashboardHandler) HandleBinaryLint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req BinaryLintRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		http.Error(w, "invalid JSON payload", http.StatusBadRequest)
		return
	}

	res := BinaryLintResponse{}

	// Parse bad bytes if supplied
	var badBytes []byte
	if len(req.BadBytes) > 0 {
		for _, s := range req.BadBytes {
			b := coprocessor.ExtractShellcodeBytes(s)
			badBytes = append(badBytes, b...)
		}
	} else {
		badBytes = coprocessor.BadBytesStringSafe
	}

	// 1. Scan for shellcode
	extracted := coprocessor.ExtractShellcodeBytes(req.Code)
	if len(extracted) > 0 {
		scan := coprocessor.ScanBadBytes(extracted, badBytes)
		res.Shellcode = &scan
	}

	// 2. Lint x86/x64 assembly mnemonics & ABI
	osTarget := req.TargetOS
	if osTarget == "" {
		osTarget = "linux"
	}
	issues := coprocessor.LintX64Assembly(req.Code, osTarget)
	if len(issues) > 0 {
		res.Assembly = issues
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.MarshalWrite(w, res)
}

