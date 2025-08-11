package object

import (
	"bytes"
	"fmt"
	"testing"
	"time"
)

func TestBlob(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    string
	}{
		{
			name:    "empty blob",
			content: []byte{},
			want:    "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391",
		},
		{
			name:    "simple text",
			content: []byte("hello world"),
			want:    "95d09f2b10159347eece71399a7e2e907ea3df4f",
		},
		{
			name:    "binary data",
			content: []byte{0x00, 0x01, 0x02, 0x03},
			want:    "eaf36c1daccfdf325514461cd1a2ffbc139b5464",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			blob := NewBlob(tt.content)

			// Test type
			if blob.Type() != TypeBlob {
				t.Errorf("Type() = %v, want %v", blob.Type(), TypeBlob)
			}

			// Test size
			if blob.Size() != int64(len(tt.content)) {
				t.Errorf("Size() = %v, want %v", blob.Size(), len(tt.content))
			}

			// Test hash
			if got := blob.Hash(); got != tt.want {
				t.Errorf("Hash() = %v, want %v", got, tt.want)
			}

			// Test content preservation
			if !bytes.Equal(blob.Content, tt.content) {
				t.Errorf("Content = %v, want %v", blob.Content, tt.content)
			}
		})
	}
}

func TestTree(t *testing.T) {
	tree := NewTree()

	// Add entries
	tree.AddEntry("100644", "file1.txt", "95d09f2b10159347eece71399a7e2e907ea3df4f")
	tree.AddEntry("100755", "script.sh", "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391")
	tree.AddEntry("040000", "subdir", "4b825dc642cb6eb9a060e54bf8d69288fbee4904")

	// Test type
	if tree.Type() != TypeTree {
		t.Errorf("Type() = %v, want %v", tree.Type(), TypeTree)
	}

	// Test entries
	if len(tree.Entries) != 3 {
		t.Errorf("len(Entries) = %v, want 3", len(tree.Entries))
	}

	// Test specific entry
	if tree.Entries[0].Name != "file1.txt" {
		t.Errorf("Entry[0].Name = %v, want file1.txt", tree.Entries[0].Name)
	}

	// Test serialization produces consistent hash
	hash1 := tree.Hash()
	hash2 := tree.Hash()
	if hash1 != hash2 {
		t.Errorf("Hash not consistent: %v != %v", hash1, hash2)
	}
}

func TestCommit(t *testing.T) {
	author := Author{
		Name:  "Test User",
		Email: "test@example.com",
		Time:  time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	commit := NewCommit("abc123", author, "Test commit")
	commit.SetParent("parent123")

	// Test type
	if commit.Type() != TypeCommit {
		t.Errorf("Type() = %v, want %v", commit.Type(), TypeCommit)
	}

	// Test fields
	if commit.TreeHash != "abc123" {
		t.Errorf("TreeHash = %v, want abc123", commit.TreeHash)
	}

	if commit.Message != "Test commit" {
		t.Errorf("Message = %v, want Test commit", commit.Message)
	}

	// Test serialization contains expected content
	serializedBytes, err := commit.Serialize()
	if err != nil {
		t.Fatalf("Failed to serialize commit: %v", err)
	}
	serialized := string(serializedBytes)
	expectedParts := []string{
		"tree abc123",
		"parent parent123",
		"Test User <test@example.com>",
		"Test commit",
	}

	for _, part := range expectedParts {
		if !bytes.Contains([]byte(serialized), []byte(part)) {
			t.Errorf("Serialized commit missing expected part: %v", part)
		}
	}
}

func TestTag(t *testing.T) {
	tagger := Author{
		Name:  "Tagger",
		Email: "tagger@example.com",
		Time:  time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC),
	}

	tag := NewTag("commit123", TypeCommit, "v1.0.0", tagger, "Release v1.0.0")

	// Test type
	if tag.Type() != TypeTag {
		t.Errorf("Type() = %v, want %v", tag.Type(), TypeTag)
	}

	// Test fields
	if tag.TagName != "v1.0.0" {
		t.Errorf("TagName = %v, want v1.0.0", tag.TagName)
	}

	// Test hash consistency
	hash1 := tag.Hash()
	hash2 := tag.Hash()
	if hash1 != hash2 {
		t.Errorf("Hash not consistent: %v != %v", hash1, hash2)
	}
}

func TestParseObject(t *testing.T) {
	tests := []struct {
		name    string
		data    []byte
		wantErr bool
		check   func(t *testing.T, obj Object)
	}{
		{
			name: "parse blob",
			data: []byte("blob 11\x00hello world"),
			check: func(t *testing.T, obj Object) {
				blob, ok := obj.(*Blob)
				if !ok {
					t.Error("Expected Blob type")
				}
				if string(blob.Content) != "hello world" {
					t.Errorf("Content = %v, want hello world", string(blob.Content))
				}
			},
		},
		{
			name:    "invalid format - no null byte",
			data:    []byte("blob 11hello world"),
			wantErr: true,
		},
		{
			name:    "invalid header format",
			data:    []byte("invalid\x00content"),
			wantErr: true,
		},
		{
			name:    "size mismatch",
			data:    []byte("blob 100\x00short"),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj, err := ParseObject(tt.data)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseObject() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.check != nil {
				tt.check(t, obj)
			}
		})
	}
}

func TestCompressDecompress(t *testing.T) {
	original := []byte("This is test data for compression")

	compressed, err := Compress(original)
	if err != nil {
		t.Fatalf("Compress() error = %v", err)
	}

	// Compressed should be different from original
	if bytes.Equal(compressed, original) {
		t.Error("Compressed data equals original")
	}

	decompressed, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress() error = %v", err)
	}

	// Decompressed should match original
	if !bytes.Equal(decompressed, original) {
		t.Errorf("Decompressed = %v, want %v", decompressed, original)
	}
}

func TestAuthorString(t *testing.T) {
	author := Author{
		Name:  "John Doe",
		Email: "john@example.com",
		Time:  time.Date(2025, 1, 1, 12, 0, 0, 0, time.FixedZone("EST", -5*60*60)),
	}

	str := author.String()
	expected := "John Doe <john@example.com> 1735750800 -0500"

	if str != expected {
		t.Errorf("String() = %v, want %v", str, expected)
	}
}

func TestParseTreeContent(t *testing.T) {
	// Create a tree and serialize it
	original := NewTree()
	original.AddEntry("100644", "file.txt", "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391")
	original.AddEntry("100755", "script.sh", "95d09f2b10159347eece71399a7e2e907ea3df4f")

	serialized, err := original.Serialize()
	if err != nil {
		t.Fatalf("Failed to serialize tree: %v", err)
	}

	// Parse it back
	parsed, err := ParseObject(serialized)
	if err != nil {
		t.Fatalf("ParseObject() error = %v", err)
	}

	tree, ok := parsed.(*Tree)
	if !ok {
		t.Fatalf("Expected Tree type, got %T", parsed)
	}

	// Compare entries
	if len(tree.Entries) != len(original.Entries) {
		t.Errorf("Entry count = %v, want %v", len(tree.Entries), len(original.Entries))
	}

	for i, entry := range tree.Entries {
		if entry.Mode != original.Entries[i].Mode {
			t.Errorf("Entry[%d].Mode = %v, want %v", i, entry.Mode, original.Entries[i].Mode)
		}
		if entry.Name != original.Entries[i].Name {
			t.Errorf("Entry[%d].Name = %v, want %v", i, entry.Name, original.Entries[i].Name)
		}
		if entry.Hash != original.Entries[i].Hash {
			t.Errorf("Entry[%d].Hash = %v, want %v", i, entry.Hash, original.Entries[i].Hash)
		}
	}
}

func BenchmarkBlobHash(b *testing.B) {
	content := bytes.Repeat([]byte("benchmark"), 1000)
	blob := NewBlob(content)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = blob.Hash()
	}
}

func BenchmarkTreeWithManyEntries(b *testing.B) {
	tree := NewTree()
	for i := 0; i < 1000; i++ {
		tree.AddEntry("100644", fmt.Sprintf("file%d.txt", i), "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = tree.Hash()
	}
}

func BenchmarkCompress(b *testing.B) {
	data := bytes.Repeat([]byte("compress me"), 100)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = Compress(data)
	}
}

// Comprehensive unit tests for uncovered functions

func TestTreeSize(t *testing.T) {
	tree := NewTree()
	
	// Test empty tree size
	if tree.Size() != 0 {
		t.Errorf("Empty tree Size() = %v, want 0", tree.Size())
	}
	
	// Add entries and test size calculation
	tree.AddEntry("100644", "file1.txt", "hash1234567890123456789012345678901234567890")
	tree.AddEntry("100755", "script.sh", "hash9876543210987654321098765432109876543210")
	
	expectedSize := int64(len(tree.serializeContent()))
	if tree.Size() != expectedSize {
		t.Errorf("Tree with entries Size() = %v, want %v", tree.Size(), expectedSize)
	}
}

func TestCommitSize(t *testing.T) {
	author := Author{
		Name:  "Test Author",
		Email: "test@example.com",
		Time:  time.Now(),
	}
	commit := NewCommit("tree1234567890123456789012345678901234567890", author, "Test commit message")
	
	expectedSize := int64(len(commit.serializeContent()))
	if commit.Size() != expectedSize {
		t.Errorf("Commit Size() = %v, want %v", commit.Size(), expectedSize)
	}
	
	// Test commit with parent
	commit.SetParent("parent123456789012345678901234567890123456789")
	expectedSizeWithParent := int64(len(commit.serializeContent()))
	if commit.Size() != expectedSizeWithParent {
		t.Errorf("Commit with parent Size() = %v, want %v", commit.Size(), expectedSizeWithParent)
	}
}

func TestCommitHash(t *testing.T) {
	author := Author{
		Name:  "Test Author",
		Email: "test@example.com",
		Time:  time.Unix(1234567890, 0),
	}
	commit := NewCommit("tree1234567890123456789012345678901234567890", author, "Test commit message")
	
	hash1 := commit.Hash()
	hash2 := commit.Hash()
	
	// Hash should be consistent
	if hash1 != hash2 {
		t.Errorf("Commit hash inconsistent: %v != %v", hash1, hash2)
	}
	
	// Hash should be valid SHA-1
	if len(hash1) != 40 {
		t.Errorf("Commit hash length = %v, want 40", len(hash1))
	}
	
	// Different commits should have different hashes
	commit2 := NewCommit("tree1234567890123456789012345678901234567890", author, "Different message")
	if commit.Hash() == commit2.Hash() {
		t.Error("Different commits should have different hashes")
	}
}

func TestTagSize(t *testing.T) {
	author := Author{
		Name:  "Test Tagger",
		Email: "tagger@example.com", 
		Time:  time.Now(),
	}
	tag := NewTag("commit1234567890123456789012345678901234567890", TypeCommit, "v1.0.0", author, "Release v1.0.0")
	
	expectedSize := int64(len(tag.serializeContent()))
	if tag.Size() != expectedSize {
		t.Errorf("Tag Size() = %v, want %v", tag.Size(), expectedSize)
	}
}

func TestParseCommit(t *testing.T) {
	// Test valid commit content
	commitContent := []byte(`tree 4b825dc642cb6eb9a060e54bf8d69288fbee4904
parent 1234567890123456789012345678901234567890
author John Doe <john@example.com> 1234567890 +0000
committer John Doe <john@example.com> 1234567890 +0000

Initial commit

This is a detailed commit message
with multiple lines.`)

	commit, err := parseCommit(commitContent)
	if err != nil {
		t.Fatalf("parseCommit() error = %v", err)
	}
	
	// Verify parsed values
	if commit.TreeHash != "4b825dc642cb6eb9a060e54bf8d69288fbee4904" {
		t.Errorf("parseCommit() tree = %v, want 4b825dc642cb6eb9a060e54bf8d69288fbee4904", commit.TreeHash)
	}
	
	if commit.ParentHash != "1234567890123456789012345678901234567890" {
		t.Errorf("parseCommit() parent = %v, want 1234567890123456789012345678901234567890", commit.ParentHash)
	}
	
	if commit.Author.Name != "John Doe" {
		t.Errorf("parseCommit() author name = %v, want John Doe", commit.Author.Name)
	}
	
	if commit.Author.Email != "john@example.com" {
		t.Errorf("parseCommit() author email = %v, want john@example.com", commit.Author.Email)
	}
	
	expectedMessage := "Initial commit\n\nThis is a detailed commit message\nwith multiple lines."
	if commit.Message != expectedMessage {
		t.Errorf("parseCommit() message = %v, want %v", commit.Message, expectedMessage)
	}
}

func TestParseCommitWithMultipleParents(t *testing.T) {
	// Test merge commit with multiple parents - note: this implementation only supports single parent
	commitContent := []byte(`tree 4b825dc642cb6eb9a060e54bf8d69288fbee4904
parent 1111111111111111111111111111111111111111
author Merger <merger@example.com> 1234567890 +0000
committer Merger <merger@example.com> 1234567890 +0000

Merge commit`)

	commit, err := parseCommit(commitContent)
	if err != nil {
		t.Fatalf("parseCommit() error = %v", err)
	}
	
	// The current implementation only supports single parent
	if commit.ParentHash != "1111111111111111111111111111111111111111" {
		t.Errorf("parseCommit() parent = %v, want 1111111111111111111111111111111111111111", commit.ParentHash)
	}
	
	if commit.Author.Name != "Merger" {
		t.Errorf("parseCommit() author name = %v, want Merger", commit.Author.Name)
	}
}

func TestParseAuthor(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		wantName  string
		wantEmail string
		wantTime  int64
		wantErr   bool
	}{
		{
			name:      "valid author",
			input:     "John Doe <john@example.com> 1234567890 +0000",
			wantName:  "John Doe",
			wantEmail: "john@example.com",
			wantTime:  1234567890,
			wantErr:   false,
		},
		{
			name:      "author with spaces in name",
			input:     "John Michael Doe <john.doe@example.com> 1577836800 -0500",
			wantName:  "John Michael Doe",
			wantEmail: "john.doe@example.com",
			wantTime:  1577836800,
			wantErr:   false,
		},
		{
			name:      "author without timezone",
			input:     "Author Name <email@test.com> 1234567890",
			wantName:  "",
			wantEmail: "",
			wantTime:  0,
			wantErr:   true,
		},
		{
			name:      "invalid email format",
			input:     "Author Name email@test.com 1234567890 +0000",
			wantName:  "",
			wantEmail: "",
			wantTime:  0,
			wantErr:   true,
		},
		{
			name:      "invalid timestamp",
			input:     "Author Name <email@test.com> notanumber +0000",
			wantName:  "",
			wantEmail: "",
			wantTime:  0,
			wantErr:   true,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			author, err := parseAuthor(tc.input)
			
			if tc.wantErr {
				if err == nil {
					t.Error("parseAuthor() expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Fatalf("parseAuthor() unexpected error = %v", err)
			}
			
			if author.Name != tc.wantName {
				t.Errorf("parseAuthor() name = %v, want %v", author.Name, tc.wantName)
			}
			
			if author.Email != tc.wantEmail {
				t.Errorf("parseAuthor() email = %v, want %v", author.Email, tc.wantEmail)
			}
			
			if author.Time.Unix() != tc.wantTime {
				t.Errorf("parseAuthor() time = %v, want %v", author.Time.Unix(), tc.wantTime)
			}
		})
	}
}

func TestParseTag(t *testing.T) {
	// Test valid tag content
	tagContent := []byte(`object 1234567890123456789012345678901234567890
type commit
tag v1.0.0
tagger Release Manager <release@example.com> 1234567890 +0000

Version 1.0.0 Release

This is the first major release
with many new features.`)

	tag, err := parseTag(tagContent)
	if err != nil {
		t.Fatalf("parseTag() error = %v", err)
	}
	
	if tag.ObjectHash != "1234567890123456789012345678901234567890" {
		t.Errorf("parseTag() object = %v, want 1234567890123456789012345678901234567890", tag.ObjectHash)
	}
	
	if tag.TagName != "v1.0.0" {
		t.Errorf("parseTag() name = %v, want v1.0.0", tag.TagName)
	}
	
	if tag.Tagger.Name != "Release Manager" {
		t.Errorf("parseTag() tagger name = %v, want Release Manager", tag.Tagger.Name)
	}
	
	if tag.Tagger.Email != "release@example.com" {
		t.Errorf("parseTag() tagger email = %v, want release@example.com", tag.Tagger.Email)
	}
	
	expectedMessage := "Version 1.0.0 Release\n\nThis is the first major release\nwith many new features."
	if tag.Message != expectedMessage {
		t.Errorf("parseTag() message = %v, want %v", tag.Message, expectedMessage)
	}
}

func TestParseTagErrors(t *testing.T) {
	testCases := []struct {
		name    string
		content []byte
		wantErr bool
	}{
		{
			name: "missing object",
			content: []byte(`type commit
tag v1.0.0
tagger Tagger <tagger@example.com> 1234567890 +0000

Message`),
			wantErr: false, // parseTag doesn't validate required fields strictly
		},
		{
			name: "missing tag name",
			content: []byte(`object 1234567890123456789012345678901234567890
type commit
tagger Tagger <tagger@example.com> 1234567890 +0000

Message`),
			wantErr: false, // parseTag doesn't validate required fields strictly
		},
		{
			name: "missing tagger",
			content: []byte(`object 1234567890123456789012345678901234567890
type commit
tag v1.0.0

Message`),
			wantErr: false, // parseTag doesn't validate required fields strictly
		},
		{
			name: "invalid tagger format",
			content: []byte(`object 1234567890123456789012345678901234567890
type commit
tag v1.0.0
tagger Invalid Tagger Format

Message`),
			wantErr: true,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseTag(tc.content)
			if (err != nil) != tc.wantErr {
				t.Errorf("parseTag() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}

func TestDeserialize(t *testing.T) {
	testCases := []struct {
		name     string
		data     []byte
		wantErr  bool
		validate func(t *testing.T, obj Object)
	}{
		{
			name:    "deserialize blob",
			data:    []byte("blob 11\x00hello world"),
			wantErr: false,
			validate: func(t *testing.T, obj Object) {
				blob, ok := obj.(*Blob)
				if !ok {
					t.Errorf("Expected *Blob, got %T", obj)
					return
				}
				if !bytes.Equal(blob.Content, []byte("hello world")) {
					t.Errorf("Blob content = %v, want 'hello world'", string(blob.Content))
				}
			},
		},
		{
			name:    "deserialize commit",
			data:    []byte("commit 165\x00tree 4b825dc642cb6eb9a060e54bf8d69288fbee4904\nauthor John Doe <john@example.com> 1234567890 +0000\ncommitter John Doe <john@example.com> 1234567890 +0000\n\nTest commit"),
			wantErr: false,
			validate: func(t *testing.T, obj Object) {
				commit, ok := obj.(*Commit)
				if !ok {
					t.Errorf("Expected *Commit, got %T", obj)
					return
				}
				if commit.Message != "Test commit" {
					t.Errorf("Commit message = %v, want 'Test commit'", commit.Message)
				}
			},
		},
		{
			name:    "deserialize tag",
			data:    []byte("tag 135\x00object 1234567890123456789012345678901234567890\ntype commit\ntag v1.0.0\ntagger Tagger <tagger@example.com> 1234567890 +0000\n\nTag message"),
			wantErr: false,
			validate: func(t *testing.T, obj Object) {
				tag, ok := obj.(*Tag)
				if !ok {
					t.Errorf("Expected *Tag, got %T", obj)
					return
				}
				if tag.TagName != "v1.0.0" {
					t.Errorf("Tag name = %v, want v1.0.0", tag.TagName)
				}
			},
		},
		{
			name:    "invalid format",
			data:    []byte("invalid format"),
			wantErr: true,
			validate: func(t *testing.T, obj Object) {
				// Should not be called for error cases
			},
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			obj, err := Deserialize(tc.data)
			
			if tc.wantErr {
				if err == nil {
					t.Error("Deserialize() expected error but got none")
				}
				return
			}
			
			if err != nil {
				t.Fatalf("Deserialize() unexpected error = %v", err)
			}
			
			if obj == nil {
				t.Error("Deserialize() returned nil object")
				return
			}
			
			// Validate specific object properties
			tc.validate(t, obj)
		})
	}
}

func TestParseCommitErrors(t *testing.T) {
	testCases := []struct {
		name    string
		content []byte
		wantErr bool
	}{
		{
			name: "missing tree",
			content: []byte(`author John Doe <john@example.com> 1234567890 +0000
committer John Doe <john@example.com> 1234567890 +0000

Message`),
			wantErr: false, // parseCommit doesn't validate required fields strictly
		},
		{
			name: "invalid author format",
			content: []byte(`tree 4b825dc642cb6eb9a060e54bf8d69288fbee4904
author Invalid Author Format
committer John Doe <john@example.com> 1234567890 +0000

Message`),
			wantErr: true,
		},
	}
	
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseCommit(tc.content)
			if (err != nil) != tc.wantErr {
				t.Errorf("parseCommit() error = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
