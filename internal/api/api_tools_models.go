package api

// --- Shared Models ---

// APIToolsError represents an error response from the API tools service.
// Only embedded inside per-endpoint Response structs (never returned standalone).
// The variant nature (`null` on success, populated on failure) is documented in
// each handler's `@Description` — swag drops sibling-of-$ref descriptions, so
// the convention can't live on the field tag. Examples are intentionally
// zero/empty so the composited success Example renders `{code:0, message:" "}`
// instead of the literal `"string"` swag falls back to with no example set.
type APIToolsError struct {
	Code    int    `json:"code" example:"0"`
	Message string `json:"message" example:" "`
}

// ProxyErrorResponse is what the reverse proxy writes when it cannot reach the
// upstream api-tools service (502 Bad Gateway). Upstream-originated errors are
// passed through unchanged and follow APIToolsError instead.
type ProxyErrorResponse struct {
	Error string `json:"error" example:"Failed to contact external API" description:"Human-readable reason the upstream call failed"`
}

// --- Mark Accent Models ---

// MarkAccentRequest is the request body for the mark-accent endpoints.
// Shared by /v1/mark-accent and /v1/mark-accent/stream.
type MarkAccentRequest struct {
	Text                   string `json:"text" example:"お金を稼ぐ" description:"Japanese text to analyze for accent patterns"`
	RenderEnglishFurigana  bool   `json:"render_english_furigana,omitempty" example:"false" description:"Emit furigana for ASCII-letter tokens (e.g. Apple→アップル). Default false."`
	RenderKatakanaFurigana bool   `json:"render_katakana_furigana,omitempty" example:"false" description:"Emit furigana for pure-katakana tokens. Default false; per-mora pitch is returned regardless."`
	Script                 string `json:"script,omitempty" example:"hiragana" enums:"hiragana,katakana,romaji" description:"Output script for every furigana field. Default hiragana. Romaji uses Hepburn-style (no macrons)."`
}

// AccentInfo represents accent marking for a single mora.
type AccentInfo struct {
	Furigana          string `json:"furigana" example:"か" description:"Furigana for this mora"`
	AccentMarkingType int    `json:"accent_marking_type" example:"1" description:"Accent type: 0=low/unknown, 1=heiban (high plateau), 2=fall kernel"`
	Length            int    `json:"length" example:"1" description:"Character length of this mora"`
}

// WordAccentSubword represents sub-word decomposition within an accent result.
// The recursive `subword` field is hidden from Swagger (`swaggerignore`) because
// it is "reserved for compatibility; typically empty" in real responses, and
// swag renders self-referential types as the literal `["string"]` which is
// worse than not documenting them. The runtime JSON still carries the field.
type WordAccentSubword struct {
	Furigana          string              `json:"furigana" example:"かね" description:"Furigana for this sub-word"`
	Surface           string              `json:"surface" example:"金" description:"Original text for this sub-word"`
	Subword           []WordAccentSubword `json:"subword,omitempty" swaggerignore:"true"`
	LexicalKernel     *int                `json:"lexical_kernel,omitempty" example:"0" description:"UniDic per-morpheme kernel position (aType). 0=heiban, N>=1=kernel on mora N. null when unavailable."`
	LexicalKernelAlts []int               `json:"lexical_kernel_alts,omitempty" description:"Alternative kernel positions for multi-reading entries"`
}

// WordAccentResult represents a single word result from the mark-accent endpoint.
type WordAccentResult struct {
	Furigana          string              `json:"furigana" example:"おかね" description:"Complete furigana for the word"`
	Surface           string              `json:"surface" example:"お金" description:"Original text as it appears in the input"`
	Accent            []AccentInfo        `json:"accent" description:"Per-mora accent marking"`
	Subword           []WordAccentSubword `json:"subword,omitempty" description:"Sub-word decomposition when a word contains both kanji and kana"`
	LexicalKernel     *int                `json:"lexical_kernel,omitempty" example:"0" description:"UniDic per-morpheme kernel position (aType). 0=heiban, N>=1=kernel on mora N. null when unavailable."`
	LexicalKernelAlts []int               `json:"lexical_kernel_alts,omitempty" description:"Alternative kernel positions for multi-reading entries (e.g. aType=\"2,0\" → [2, 0])"`
	KernelAbsorbed    bool                `json:"kernel_absorbed,omitempty" example:"false" description:"True when UniDic reports a kernel but OJAD's surface contour absorbed it into a larger prosodic phrase"`
}

// MarkAccentResponse is the response from the mark-accent endpoint.
type MarkAccentResponse struct {
	Status int                `json:"status" example:"200" description:"HTTP status code"`
	Result []WordAccentResult `json:"result" extensions:"x-nullable=true" description:"List of accent-marked word results; null when status != 200"`
	Error  *APIToolsError     `json:"error"`
}

// MarkAccentStreamChunk is one NDJSON line emitted by /v1/mark-accent/stream.
// The full response is a stream of these objects, one per line, separated by '\n'.
type MarkAccentStreamChunk struct {
	Chunk    int                `json:"chunk" example:"0" description:"Zero-based index of the input line this chunk belongs to"`
	Subchunk int                `json:"subchunk" example:"0" description:"Zero-based sub-chunk index within the line"`
	Status   int                `json:"status" example:"200" description:"HTTP status code for this chunk"`
	Result   []WordAccentResult `json:"result" extensions:"x-nullable=true" description:"List of accent-marked word results for this chunk; null when this chunk's status != 200"`
	Error    *APIToolsError     `json:"error"`
}

// --- Usage Query: HeadWords Models ---

// UsageQueryHeadWordsRequest is the request body for the usage-query/headwords endpoint.
type UsageQueryHeadWordsRequest struct {
	Word string `json:"word" example:"走る" description:"Word to query (kanji, kana, or romaji)"`
	Site string `json:"site" example:"NLB" description:"Corpus site to query: NLB (NINJAL LRP BCCWJ) or NLT (Tsukuba Web Corpus)"`
}

// HeadWord represents a single headword result.
type HeadWord struct {
	ID            int    `json:"id" example:"1" description:"Numeric ID of the headword"`
	HeadwordID    string `json:"headword_id" example:"V.00093" description:"Headword identifier used for detail queries"`
	Headword      string `json:"headword" example:"走る" description:"The headword in kanji"`
	YomiDisplay   string `json:"yomi_display" example:"ハシ・ル" description:"Reading in katakana"`
	RomajiDisplay string `json:"romaji_display" example:"hashiru" description:"Romaji representation"`
	Freq          int    `json:"freq" example:"1234" description:"Frequency in the corpus"`
}

// UsageQueryHeadWordsResponse is the response from the usage-query/headwords endpoint.
type UsageQueryHeadWordsResponse struct {
	Status int            `json:"status" example:"200" description:"HTTP status code"`
	Result []HeadWord     `json:"result" extensions:"x-nullable=true" description:"List of matching headwords; null when status != 200"`
	Error  *APIToolsError `json:"error"`
}

// --- Usage Query: URL Models ---

// UsageQueryURLResponse is the response from the usage-query/url endpoint.
type UsageQueryWordURL struct {
	Word string `json:"word" example:"走る" description:"The headword"`
	URL  string `json:"url" example:"https://nlb.ninjal.ac.jp/headword/V.00093/" description:"URL to the headword detail page"`
}

// UsageQueryURLResponse is the response from the usage-query/url endpoint.
type UsageQueryURLResponse struct {
	Status int                 `json:"status" example:"200" description:"HTTP status code"`
	Result []UsageQueryWordURL `json:"result" extensions:"x-nullable=true" description:"List of headword URLs; null when status != 200"`
	Error  *APIToolsError      `json:"error"`
}

// --- Usage Query: IdDetails Models ---

// UsageQueryIDDetailsRequest is the request body for the usage-query/id-details endpoint.
type UsageQueryIDDetailsRequest struct {
	HeadwordID string `json:"headword_id" example:"V.00093" description:"Headword ID obtained from the headwords endpoint"`
	Site       string `json:"site" example:"NLB" description:"Corpus site: NLB or NLT"`
}

// IdDetails contains detailed word usage information.
type IdDetails struct {
	Base              []map[string]any `json:"base" description:"Base form information"`
	Subcorpus         []map[string]any `json:"subcorpus" description:"Subcorpus distribution"`
	Shojikei          []map[string]any `json:"shojikei" description:"Shojikei (conjugation types)"`
	SubcorpusShojikei []map[string]any `json:"subcorpus_shojikei" description:"Shojikei distribution by subcorpus"`
	Katuyokei         []map[string]any `json:"katuyokei" description:"Katuyokei (conjugation forms)"`
	Setuzoku          []map[string]any `json:"setuzoku" description:"Subsequent auxiliary verbs"`
	Patternfreqorder  []map[string]any `json:"patternfreqorder" description:"Frequency in different patterns"`
}

// UsageQueryIDDetailsResponse is the response from the usage-query/id-details endpoint.
type UsageQueryIDDetailsResponse struct {
	Status int            `json:"status" example:"200" description:"HTTP status code"`
	Result *IdDetails     `json:"result" extensions:"x-nullable=true" description:"Detailed word usage data; null when status != 200"`
	Error  *APIToolsError `json:"error"`
}

// --- Dictionary Query Models ---

// DictQueryRequest is the request body for the dict-query endpoint.
type DictQueryRequest struct {
	Word string `json:"word" example:"先生" description:"Word to look up in the dictionary"`
}

// Definition represents a single sense/definition of a dictionary word.
type Definition struct {
	Pos      []string `json:"pos" example:"noun" description:"Part-of-speech tags"`
	Meanings []string `json:"meanings" example:"teacher" description:"English meanings of the word"`
}

// DictQueryWordResult represents a single dictionary word result.
type DictQueryWordResult struct {
	Kanji       []string     `json:"kanji" example:"先生" description:"Kanji representations"`
	Furigana    []string     `json:"furigana" example:"せんせい" description:"Furigana readings"`
	Definitions []Definition `json:"definitions" description:"Word definitions with part-of-speech tags"`
	ID          int          `json:"id" example:"1387990" description:"JMdict entry ID"`
}

// DictQueryResponse is the response from the dict-query endpoint.
type DictQueryResponse struct {
	Status int                   `json:"status" example:"200" description:"HTTP status code"`
	Result []DictQueryWordResult `json:"result" extensions:"x-nullable=true" description:"List of dictionary word results; null when status != 200"`
	Error  *APIToolsError        `json:"error"`
}

// --- Sentence Query Models ---

// SentenceQueryRequest is the request body for the sentence-query endpoint.
type SentenceQueryRequest struct {
	Word string `json:"word" example:"先生" description:"Word (kanji or furigana) to find example sentences for"`
	ID   int    `json:"id" example:"1387990" description:"JMdict entry ID obtained from the dict-query endpoint"`
}

// WordSentence represents a bilingual sentence pair.
type WordSentence struct {
	JP string `json:"jp" example:"先生に聞いてみます。" description:"Japanese example sentence"`
	EN string `json:"en" example:"I will ask the teacher." description:"English translation"`
}

// SentenceQueryWordResult represents the sentence query result for a word.
type SentenceQueryWordResult struct {
	Word     string         `json:"word" example:"先生" description:"The queried word"`
	ID       int            `json:"id" example:"1387990" description:"JMdict entry ID"`
	Sentence []WordSentence `json:"sentence" description:"List of bilingual example sentences"`
}

// SentenceQueryResponse is the response from the sentence-query endpoint.
type SentenceQueryResponse struct {
	Status int                      `json:"status" example:"200" description:"HTTP status code"`
	Result *SentenceQueryWordResult `json:"result" extensions:"x-nullable=true" description:"Sentence query results for the word; null when status != 200"`
	Error  string                   `json:"error"`
}
