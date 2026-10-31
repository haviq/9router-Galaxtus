package dashboard

import (
	"9router/proxy/internal/db"
	"9router/proxy/internal/handlerutil"
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// SYADAGENTIC pack import. The Sekolah76/syadagentic collection is not one file
// per model — it is a single prompt-injection/templates.md holding 40 numbered
// techniques (T1..T119), each a "## T<N>. TITLE" section. Importing it through
// the generic GitHub importer would store one preset named after the file, which
// is useless: the value is in the sections. This file parses them out, so one
// click lands the whole pack in the preset library, one preset per technique.
//
// The section text is stored verbatim as the preset prompt. The technique's
// number and title become the preset name, and each preset is labelled with a
// kind so the library groups them. Keywords are deliberately left empty: a
// technique is not bound to a model by name, so arming one stays an operator
// decision on the Prompt Injection page rather than a guess made here.

// syadagenticRepo is the collection this pack is sourced from. Fixed rather
// than operator-supplied: the pack endpoint is a convenience for one known
// collection, and a caller who wants an arbitrary repo still has the generic
// import.
const syadagenticRepo = "Sekolah76/syadagentic"

// syadagenticTemplatesPath is the file inside the repository holding the pack.
const syadagenticTemplatesPath = "prompt-injection/templates.md"

// syadagenticKind labels every preset the pack writes, so the library can tell
// a SYADAGENTIC technique from an operator's own prompt.
const syadagenticKind = "SYADAGENTIC"

// syadagenticHeading matches a technique heading: "## T104. DAN (Do Anything
// Now) ROLEPLAY FRAMING". The number is kept as-is (T1, T104, T119) because it
// is what the collection itself uses to refer to a technique.
var syadagenticHeading = regexp.MustCompile(`^##\s+(T\d+)\.\s*(.+?)\s*$`)

// syadagenticSection is one parsed technique.
type syadagenticSection struct {
	Number string // "T104"
	Title  string // "DAN (Do Anything Now) ROLEPLAY FRAMING"
	Body   string // the section text, heading excluded
}

// parseSyadagenticTemplates splits templates.md into one section per technique.
// Content before the first heading (the file's own preamble and MODEL MATRIX)
// is dropped: it describes the collection, it is not a technique. A section ends
// at the next "## " heading, so "### " subheadings stay inside their technique.
func parseSyadagenticTemplates(md string) []syadagenticSection {
	md = strings.ReplaceAll(md, "\r\n", "\n")
	lines := strings.Split(md, "\n")

	var (
		out     []syadagenticSection
		current *syadagenticSection
		body    []string
	)
	flush := func() {
		if current == nil {
			return
		}
		current.Body = strings.TrimSpace(strings.Join(body, "\n"))
		if current.Body != "" {
			out = append(out, *current)
		}
	}
	for _, line := range lines {
		if m := syadagenticHeading.FindStringSubmatch(line); m != nil {
			flush()
			current = &syadagenticSection{Number: m[1], Title: m[2]}
			body = body[:0]
			continue
		}
		// Any other top-level "## " heading ends the current technique and is
		// not itself one (MODEL MATRIX, "CARA PAKAI", appendices).
		if strings.HasPrefix(line, "## ") {
			flush()
			current = nil
			body = body[:0]
			continue
		}
		if current != nil {
			body = append(body, line)
		}
	}
	flush()
	return out
}

// syadagenticPreset turns one parsed section into a preset. The body carries
// everything the technique needs, so the stored prompt is the section as-is;
// the number and title only name and label it.
func syadagenticPreset(s syadagenticSection, source string) *db.JailbreakDoc {
	name := s.Number + " · " + s.Title
	return &db.JailbreakDoc{
		ID:       slugify("syadagentic-" + s.Number),
		Name:     name,
		Prompt:   s.Body,
		Kind:     syadagenticKind,
		Source:   source,
		Keywords: nil,
	}
}

// syadagenticImportResult is the pack import verdict, shaped like the generic
// import so the dashboard renders both the same way.
type syadagenticImportResult struct {
	Source   string                   `json:"source"`
	Imported []jailbreakImportEntry   `json:"imported"`
	Refused  []jailbreakImportRefusal `json:"refused"`
}

// importSyadagenticPack fetches templates.md, parses it, and writes one preset
// per technique. A section whose id already exists is overwritten, so
// re-running the import refreshes the pack in place instead of duplicating it.
func (imp *jailbreakImporter) importSyadagenticPack(ctx context.Context, repo *db.Repo) (*syadagenticImportResult, error) {
	src := githubJailbreakSource{Owner: "Sekolah76", Repo: "syadagentic"}
	ref, err := imp.defaultBranch(ctx, src)
	if err != nil {
		return nil, err
	}
	rawURL := imp.rawURL(src, ref, syadagenticTemplatesPath)
	md, err := imp.fetchText(ctx, rawURL, maxJailbreakImportFileBytes)
	if err != nil {
		return nil, err
	}
	sections := parseSyadagenticTemplates(md)
	if len(sections) == 0 {
		return nil, fmt.Errorf("no T-numbered sections found in %s", syadagenticTemplatesPath)
	}

	source := fmt.Sprintf("%s/%s@%s/%s", "Sekolah76", "syadagentic", ref, syadagenticTemplatesPath)
	res := &syadagenticImportResult{
		Source:   source,
		Imported: make([]jailbreakImportEntry, 0, len(sections)),
		Refused:  make([]jailbreakImportRefusal, 0),
	}
	for _, s := range sections {
		doc := syadagenticPreset(s, source)
		if err := repo.SaveJailbreakPreset(doc); err != nil {
			res.Refused = append(res.Refused, jailbreakImportRefusal{
				Path:   s.Number,
				Reason: err.Error(),
			})
			continue
		}
		res.Imported = append(res.Imported, jailbreakImportEntry{
			ID:       doc.ID,
			Name:     doc.Name,
			Path:     s.Number,
			Kind:     doc.Kind,
			Keywords: nil,
		})
	}
	if len(res.Imported) == 0 {
		return nil, fmt.Errorf("every section in %s was refused", syadagenticTemplatesPath)
	}
	return res, nil
}

// HandleImportSyadagenticPack installs the SYADAGENTIC technique pack as
// presets. POST /api/jailbreak/import/syadagentic, no body.
func (h *DashboardHandler) HandleImportSyadagenticPack(w http.ResponseWriter, r *http.Request) {
	res, err := h.importer().importSyadagenticPack(r.Context(), h.Repo)
	if err != nil {
		handlerutil.WriteJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	handlerutil.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"source":   res.Source,
		"imported": res.Imported,
		"refused":  res.Refused,
	})
}
