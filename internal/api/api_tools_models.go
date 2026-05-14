package api

// --- Shared Models ---

// APIToolsError represents an error response from the API tools service.
type APIToolsError struct {
	Code    int    `json:"code" example:"500" description:"Error code following JSON-RPC 2.0 specification"`
	Message string `json:"message" example:"HTTP error 500" description:"Detailed error message"`
}

// --- Mark Accent Models ---

// MarkAccentRequest is the request body for the mark-accent endpoint.
type MarkAccentRequest struct {
	Text string `json:"text" example:"お金を稼ぐ" description:"Japanese text to analyze for accent patterns"`
}

// AccentInfo represents accent marking for a single mora.
type AccentInfo struct {
	Furigana          string `json:"furigana" example:"か" description:"Furigana for this mora"`
	AccentMarkingType int    `json:"accent_marking_type" example:"1" description:"Accent type: 0=no accent, 1=heiban (flat), 2=fall down"`
	Length            int    `json:"length" example:"1" description:"Character length of this mora"`
}

// WordAccentSubword represents sub-word decomposition within an accent result.
type WordAccentSubword struct {
	Furigana string `json:"furigana" example:"かね" description:"Furigana for this sub-word"`
	Surface  string `json:"surface" example:"金" description:"Original text for this sub-word"`
}

// WordAccentResult represents a single word result from the mark-accent endpoint.
type WordAccentResult struct {
	Furigana string            `json:"furigana" example:"おかね" description:"Complete furigana for the word"`
	Surface  string            `json:"surface" example:"お金" description:"Original text as it appears in the input"`
	Accent   []AccentInfo      `json:"accent" description:"Accent marking for each mora"`
	Subword  []WordAccentSubword `json:"subword" description:"Sub-word decomposition when a word contains both kanji and kana"`
}

// MarkAccentResponse is the response from the mark-accent endpoint.
type MarkAccentResponse struct {
	Status int                `json:"status" example:"200" description:"HTTP status code"`
	Result []WordAccentResult `json:"result" description:"List of accent-marked word results"`
	Error  *APIToolsError     `json:"error" description:"Error details, if any"`
}

// --- Mark Furigana Models ---

// MarkFuriganaRequest is the request body for the mark-furigana endpoint.
type MarkFuriganaRequest struct {
	Text string `json:"text" example:"漢字かな交じり文" description:"Japanese text to annotate with furigana"`
}

// WordResult represents a single word result from the mark-furigana endpoint.
type WordResult struct {
	Furigana string        `json:"furigana" example:"かんじ" description:"Furigana reading for the word"`
	Surface  string        `json:"surface" example:"漢字" description:"Original text as it appears in the input"`
	Subword  []WordResult  `json:"subword" description:"Sub-word decomposition when a word contains both kanji and kana"`
}

// MarkFuriganaResponse is the response from the mark-furigana endpoint.
type MarkFuriganaResponse struct {
	Status int            `json:"status" example:"200" description:"HTTP status code"`
	Result []WordResult   `json:"result" description:"List of furigana-annotated word results"`
	Error  *APIToolsError `json:"error" description:"Error details, if any"`
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
	Result []HeadWord     `json:"result" description:"List of matching headwords"`
	Error  *APIToolsError `json:"error" description:"Error details, if any"`
}

// --- Usage Query: URL Models ---

// UsageQueryURLResponse is the response from the usage-query/url endpoint.
type UsageQueryWordURL struct {
	Word string `json:"word" example:"走る" description:"The headword"`
	URL  string `json:"url" example:"https://nlb.ninjal.ac.jp/headword/V.00093/" description:"URL to the headword detail page"`
}

// UsageQueryURLResponse is the response from the usage-query/url endpoint.
type UsageQueryURLResponse struct {
	Status int                `json:"status" example:"200" description:"HTTP status code"`
	Result []UsageQueryWordURL `json:"result" description:"List of headword URLs"`
	Error  *APIToolsError      `json:"error" description:"Error details, if any"`
}

// --- Usage Query: IdDetails Models ---

// UsageQueryIDDetailsRequest is the request body for the usage-query/id-details endpoint.
type UsageQueryIDDetailsRequest struct {
	HeadwordID string `json:"headword_id" example:"V.00093" description:"Headword ID obtained from the headwords endpoint"`
	Site       string `json:"site" example:"NLB" description:"Corpus site: NLB or NLT"`
}

// IdDetails contains detailed word usage information.
type IdDetails struct {
	Base               []map[string]any `json:"base" description:"Base form information"`
	Subcorpus          []map[string]any `json:"subcorpus" description:"Subcorpus distribution"`
	Shojikei           []map[string]any `json:"shojikei" description:"Shojikei (conjugation types)"`
	SubcorpusShojikei  []map[string]any `json:"subcorpus_shojikei" description:"Shojikei distribution by subcorpus"`
	Katuyokei          []map[string]any `json:"katuyokei" description:"Katuyokei (conjugation forms)"`
	Setuzoku           []map[string]any `json:"setuzoku" description:"Subsequent auxiliary verbs"`
	Patternfreqorder   []map[string]any `json:"patternfreqorder" description:"Frequency in different patterns"`
}

// UsageQueryIDDetailsResponse is the response from the usage-query/id-details endpoint.
type UsageQueryIDDetailsResponse struct {
	Status int            `json:"status" example:"200" description:"HTTP status code"`
	Result *IdDetails      `json:"result" description:"Detailed word usage data"`
	Error  *APIToolsError `json:"error" description:"Error details, if any"`
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
	Status int                  `json:"status" example:"200" description:"HTTP status code"`
	Result []DictQueryWordResult `json:"result" description:"List of dictionary word results"`
	Error  *APIToolsError        `json:"error" description:"Error details, if any"`
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
	Status int                     `json:"status" example:"200" description:"HTTP status code"`
	Result *SentenceQueryWordResult `json:"result" description:"Sentence query results for the word"`
	Error  string                  `json:"error" description:"Error message, if any"`
}