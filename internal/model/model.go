package model

// Practice represents the jpcorrect.practice table
type Practice struct {
	PracticeID int `db:"practice_id" json:"practice_id"`
	UserID     int `db:"user_id" json:"user_id"`
}

// Error represents the jpcorrect.error table
type Error struct {
	ErrorID        int     `db:"error_id" json:"error_id"`
	PracticeID     int     `db:"practice_id" json:"practice_id"`
	UserID         int     `db:"user_id" json:"user_id"`
	ErrorType      string  `db:"error_type" json:"error_type"`
	AIDetected     bool    `db:"ai_detected" json:"ai_detected"`
	AIMiscorrected bool    `db:"ai_miscorrected" json:"ai_miscorrected"`
	HumanCorrected bool    `db:"human_corrected" json:"human_corrected"`
	StartTime      float64 `db:"start_time" json:"start_time"`
	EndTime        float64 `db:"end_time" json:"end_time"`
}

// AICorrection represents the jpcorrect.ai_correction table
type AICorrection struct {
	AICorrectionID int    `db:"ai_correction_id" json:"ai_correction_id"`
	ErrorID        int    `db:"error_id" json:"error_id"`
	Content        string `db:"content" json:"content"`
}

// Note represents the jpcorrect.note table
type Note struct {
	NoteID     int    `db:"note_id" json:"note_id"`
	PracticeID int    `db:"practice_id" json:"practice_id"`
	Content    string `db:"content" json:"content"`
}

// Transcript represents the jpcorrect.transcript table
type Transcript struct {
	TranscriptID int    `db:"transcript_id" json:"transcript_id"`
	ErrorID      int    `db:"error_id" json:"error_id"`
	Content      string `db:"content" json:"content"`
	Furigana     string `db:"furigana" json:"furigana"`
	Accent       string `db:"accent" json:"accent"`
}
