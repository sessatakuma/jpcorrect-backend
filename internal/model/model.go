package model

// Practice represents the jpcorrect.practice table
type Practice struct {
	PracticeID int     `db:"practice_id" json:"practice_id"`
	StartTime  float64 `db:"start_time" json:"start_time"`
	EndTime    float64 `db:"end_time" json:"end_time"`
}

// ErrorTag represents the jpcorrect.error_tag table
type ErrorTag struct {
	ErrorTagID     int    `db:"error_tag_id" json:"error_tag_id"`
	PracticeID     int    `db:"practice_id" json:"practice_id"`
	ErrorPersonID  int    `db:"error_person_id" json:"error_person_id"`
	ErrorType      string `db:"error_type" json:"error_type"`
	AIFlag         bool   `db:"ai_flag" json:"ai_flag"`
	AICorrected    bool   `db:"ai_corrected" json:"ai_corrected"`
	HumanCorrected bool   `db:"human_corrected" json:"human_corrected"`
}

// AICorrection represents the jpcorrect.ai_correction table
type AICorrection struct {
	AICorrectionID    int    `db:"ai_correction_id" json:"ai_correction_id"`
	ErrorTagID        int    `db:"error_tag_id" json:"error_tag_id"`
	CorrectionContent string `db:"correction_content" json:"correction_content"`
}

// Note represents the jpcorrect.note table
type Note struct {
	NoteID     int    `db:"note_id" json:"note_id"`
	PracticeID int    `db:"practice_id" json:"practice_id"`
	UserNote   string `db:"user_note" json:"user_note"`
}

// XMLDetail represents the jpcorrect.xml_detail table
type XMLDetail struct {
	XMLDetailID int    `db:"xml_detail_id" json:"xml_detail_id"`
	ErrorTagID  int    `db:"error_tag_id" json:"error_tag_id"`
	TextContent string `db:"text_content" json:"text_content"`
	Furigana    string `db:"furigana" json:"furigana"`
	Pitch       string `db:"pitch" json:"pitch"`
}
