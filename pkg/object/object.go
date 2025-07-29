package object

import (
	"bytes"
	"compress/zlib"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

type Type string

const (
	TypeBlob   Type = "blob"
	TypeTree   Type = "tree"
	TypeCommit Type = "commit"
	TypeTag    Type = "tag"
)

type Object interface {
	Type() Type
	Size() int64
	Hash() string
	Serialize() []byte
}

type Blob struct {
	Content []byte
}

func NewBlob(content []byte) *Blob {
	return &Blob{Content: content}
}

func (b *Blob) Type() Type {
	return TypeBlob
}

func (b *Blob) Size() int64 {
	return int64(len(b.Content))
}

func (b *Blob) Hash() string {
	data := b.Serialize()
	return HashObject(data)
}

func (b *Blob) Serialize() []byte {
	header := fmt.Sprintf("%s %d\x00", b.Type(), b.Size())
	return append([]byte(header), b.Content...)
}

type TreeEntry struct {
	Mode string
	Name string
	Hash string
}

type Tree struct {
	Entries []TreeEntry
}

func NewTree() *Tree {
	return &Tree{
		Entries: make([]TreeEntry, 0),
	}
}

func (t *Tree) AddEntry(mode, name, hash string) {
	t.Entries = append(t.Entries, TreeEntry{
		Mode: mode,
		Name: name,
		Hash: hash,
	})
}

func (t *Tree) Type() Type {
	return TypeTree
}

func (t *Tree) Size() int64 {
	content := t.serializeContent()
	return int64(len(content))
}

func (t *Tree) Hash() string {
	data := t.Serialize()
	return HashObject(data)
}

func (t *Tree) serializeContent() []byte {
	var buf bytes.Buffer
	for _, entry := range t.Entries {
		fmt.Fprintf(&buf, "%s %s\x00", entry.Mode, entry.Name)
		hash, _ := hex.DecodeString(entry.Hash)
		buf.Write(hash)
	}
	return buf.Bytes()
}

func (t *Tree) Serialize() []byte {
	content := t.serializeContent()
	header := fmt.Sprintf("%s %d\x00", t.Type(), len(content))
	return append([]byte(header), content...)
}

type Author struct {
	Name  string
	Email string
	Time  time.Time
}

func (a Author) String() string {
	timestamp := a.Time.Unix()
	tz := a.Time.Format("-0700")
	return fmt.Sprintf("%s <%s> %d %s", a.Name, a.Email, timestamp, tz)
}

type Commit struct {
	TreeHash   string
	ParentHash string
	Author     Author
	Committer  Author
	Message    string
}

func NewCommit(treeHash string, author Author, message string) *Commit {
	return &Commit{
		TreeHash:  treeHash,
		Author:    author,
		Committer: author,
		Message:   message,
	}
}

func (c *Commit) SetParent(parentHash string) {
	c.ParentHash = parentHash
}

func (c *Commit) Type() Type {
	return TypeCommit
}

func (c *Commit) Size() int64 {
	content := c.serializeContent()
	return int64(len(content))
}

func (c *Commit) Hash() string {
	data := c.Serialize()
	return HashObject(data)
}

func (c *Commit) serializeContent() []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "tree %s\n", c.TreeHash)
	if c.ParentHash != "" {
		fmt.Fprintf(&buf, "parent %s\n", c.ParentHash)
	}
	fmt.Fprintf(&buf, "author %s\n", c.Author.String())
	fmt.Fprintf(&buf, "committer %s\n", c.Committer.String())
	fmt.Fprintf(&buf, "\n%s", c.Message)
	return buf.Bytes()
}

func (c *Commit) Serialize() []byte {
	content := c.serializeContent()
	header := fmt.Sprintf("%s %d\x00", c.Type(), len(content))
	return append([]byte(header), content...)
}

type Tag struct {
	ObjectHash string
	ObjectType Type
	TagName    string
	Tagger     Author
	Message    string
}

func NewTag(objectHash string, objectType Type, tagName string, tagger Author, message string) *Tag {
	return &Tag{
		ObjectHash: objectHash,
		ObjectType: objectType,
		TagName:    tagName,
		Tagger:     tagger,
		Message:    message,
	}
}

func (t *Tag) Type() Type {
	return TypeTag
}

func (t *Tag) Size() int64 {
	content := t.serializeContent()
	return int64(len(content))
}

func (t *Tag) Hash() string {
	data := t.Serialize()
	return HashObject(data)
}

func (t *Tag) serializeContent() []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "object %s\n", t.ObjectHash)
	fmt.Fprintf(&buf, "type %s\n", t.ObjectType)
	fmt.Fprintf(&buf, "tag %s\n", t.TagName)
	fmt.Fprintf(&buf, "tagger %s\n", t.Tagger.String())
	fmt.Fprintf(&buf, "\n%s", t.Message)
	return buf.Bytes()
}

func (t *Tag) Serialize() []byte {
	content := t.serializeContent()
	header := fmt.Sprintf("%s %d\x00", t.Type(), len(content))
	return append([]byte(header), content...)
}

func HashObject(data []byte) string {
	h := sha1.New()
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

func Compress(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zlib.NewWriter(&buf)
	if _, err := w.Write(data); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func Decompress(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

func ParseObject(data []byte) (Object, error) {
	nullIndex := bytes.IndexByte(data, 0)
	if nullIndex == -1 {
		return nil, fmt.Errorf("invalid object format: no null byte")
	}

	header := string(data[:nullIndex])
	parts := strings.Split(header, " ")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid header format: %s", header)
	}

	objType := Type(parts[0])
	size, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid size: %v", err)
	}

	content := data[nullIndex+1:]
	if int64(len(content)) != size {
		return nil, fmt.Errorf("content size mismatch: expected %d, got %d", size, len(content))
	}

	switch objType {
	case TypeBlob:
		return &Blob{Content: content}, nil
	case TypeTree:
		return parseTree(content)
	case TypeCommit:
		return parseCommit(content)
	case TypeTag:
		return parseTag(content)
	default:
		return nil, fmt.Errorf("unknown object type: %s", objType)
	}
}

func parseTree(content []byte) (*Tree, error) {
	tree := NewTree()
	buf := bytes.NewBuffer(content)

	for buf.Len() > 0 {
		modeAndName, err := buf.ReadBytes(0)
		if err != nil {
			return nil, fmt.Errorf("error reading tree entry: %v", err)
		}
		modeAndName = modeAndName[:len(modeAndName)-1]

		parts := strings.SplitN(string(modeAndName), " ", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tree entry format")
		}

		hash := make([]byte, 20)
		if _, err := buf.Read(hash); err != nil {
			return nil, fmt.Errorf("error reading hash: %v", err)
		}

		tree.AddEntry(parts[0], parts[1], hex.EncodeToString(hash))
	}

	return tree, nil
}

func parseCommit(content []byte) (*Commit, error) {
	lines := strings.Split(string(content), "\n")
	commit := &Commit{}

	messageStart := 0
	for i, line := range lines {
		if line == "" {
			messageStart = i + 1
			break
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}

		switch parts[0] {
		case "tree":
			commit.TreeHash = parts[1]
		case "parent":
			commit.ParentHash = parts[1]
		case "author":
			author, err := parseAuthor(parts[1])
			if err != nil {
				return nil, fmt.Errorf("error parsing author: %v", err)
			}
			commit.Author = author
		case "committer":
			committer, err := parseAuthor(parts[1])
			if err != nil {
				return nil, fmt.Errorf("error parsing committer: %v", err)
			}
			commit.Committer = committer
		}
	}

	if messageStart < len(lines) {
		commit.Message = strings.Join(lines[messageStart:], "\n")
	}

	return commit, nil
}

func parseAuthor(authorStr string) (Author, error) {
	emailStart := strings.Index(authorStr, "<")
	emailEnd := strings.Index(authorStr, ">")
	if emailStart == -1 || emailEnd == -1 || emailStart >= emailEnd {
		return Author{}, fmt.Errorf("invalid author format")
	}

	name := strings.TrimSpace(authorStr[:emailStart])
	email := authorStr[emailStart+1 : emailEnd]
	
	timestampParts := strings.Fields(authorStr[emailEnd+1:])
	if len(timestampParts) < 2 {
		return Author{}, fmt.Errorf("invalid timestamp format")
	}

	timestamp, err := strconv.ParseInt(timestampParts[0], 10, 64)
	if err != nil {
		return Author{}, fmt.Errorf("invalid timestamp: %v", err)
	}

	return Author{
		Name:  name,
		Email: email,
		Time:  time.Unix(timestamp, 0),
	}, nil
}

func parseTag(content []byte) (*Tag, error) {
	lines := strings.Split(string(content), "\n")
	tag := &Tag{}

	messageStart := 0
	for i, line := range lines {
		if line == "" {
			messageStart = i + 1
			break
		}

		parts := strings.SplitN(line, " ", 2)
		if len(parts) != 2 {
			continue
		}

		switch parts[0] {
		case "object":
			tag.ObjectHash = parts[1]
		case "type":
			tag.ObjectType = Type(parts[1])
		case "tag":
			tag.TagName = parts[1]
		case "tagger":
			tagger, err := parseAuthor(parts[1])
			if err != nil {
				return nil, fmt.Errorf("error parsing tagger: %v", err)
			}
			tag.Tagger = tagger
		}
	}

	if messageStart < len(lines) {
		tag.Message = strings.Join(lines[messageStart:], "\n")
	}

	return tag, nil
}