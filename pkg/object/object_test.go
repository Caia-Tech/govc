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
			want:    "1c95eaa7e46f5f5d2a43107eed6c1e71df8c6b8b",
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
	serialized := string(commit.Serialize())
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
	expected := "John Doe <john@example.com> 1735740000 -0500"
	
	if str != expected {
		t.Errorf("String() = %v, want %v", str, expected)
	}
}

func TestParseTreeContent(t *testing.T) {
	// Create a tree and serialize it
	original := NewTree()
	original.AddEntry("100644", "file.txt", "e69de29bb2d1d6434b8b29ae775ad8c2e48c5391")
	original.AddEntry("100755", "script.sh", "95d09f2b10159347eece71399a7e2e907ea3df4f")

	serialized := original.Serialize()

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