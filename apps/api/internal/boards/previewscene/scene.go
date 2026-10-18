// Package previewscene owns the versioned Board Preview Scene contract.
//
// A Scene contains geometry and media identities derived from an accepted
// board snapshot. It carries no URLs, raster payloads, markup, or prompt text.
// Catalog projections publish in the document's accepting transaction.
// See contract.md for the wire contract.
package previewscene

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
)

// SceneVersion is the current PreviewScene contract version. Parsers reject
// any other version fail-closed; evolution is additive-only (see contract.md).
const SceneVersion = 1

// SnapshotSchemaVersion mirrors the board document schema version this
// extractor accepts (BoardSnapshot.schemaVersion). Snapshots from any other
// schema version are rejected, never migrated or guessed.
const SnapshotSchemaVersion = 4

// DefaultSceneTier is the media tier assumed when a snapshot media identity
// carries no usable derivative tier. The gallery thumbnail tier is
// independent of canvas zoom; it is never inferred per device.
const DefaultSceneTier = "preview"

// Catalog policy: the single source for the persisted Gallery scene
// budgets. CatalogLimits, contract.md §Budgets, and the budget tests all
// reference these values; change them here only (values are unchanged).
const (
	CatalogMaxNodes       = 500
	CatalogMaxConnections = 1000
	CatalogMaxMediaRefs   = 1500
	CatalogMaxSceneBytes  = 65536
)

var (
	// ErrEmptyContentVersion rejects extraction without a content identity.
	ErrEmptyContentVersion = errors.New("previewscene: content version is empty")
	// ErrUnsupportedSnapshot rejects snapshots outside SnapshotSchemaVersion.
	ErrUnsupportedSnapshot = errors.New("previewscene: unsupported snapshot schema version")
	// ErrInvalidSnapshot rejects snapshots that fail required-field checks.
	ErrInvalidSnapshot = errors.New("previewscene: invalid snapshot")
	// ErrInvalidScene rejects scene payloads that fail strict parsing.
	ErrInvalidScene = errors.New("previewscene: invalid scene")
	// ErrUnsupportedSceneVersion rejects scene payloads with version != SceneVersion.
	ErrUnsupportedSceneVersion = errors.New("previewscene: unsupported scene version")
)

// ScenePoint is one normalized coordinate in scene space.
type ScenePoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// SceneBounds is the full-layout bounds in board coordinates.
type SceneBounds struct {
	MinX   float64 `json:"min_x"`
	MinY   float64 `json:"min_y"`
	MaxX   float64 `json:"max_x"`
	MaxY   float64 `json:"max_y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// SceneExtent is the normalized layout extent: bounds scaled by the uniform
// factor 1/max(width,height), so exactly one side equals 1 and aspect holds.
type SceneExtent struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// SceneMediaRef is one pure media identity. At least one of Asset or
// AssetNumeric is present; URLs are never carried. Tier defaults to
// DefaultSceneTier when the snapshot provides no usable tier.
type SceneMediaRef struct {
	Asset        string `json:"asset"`
	AssetNumeric *int64 `json:"asset_numeric"`
	Tier         string `json:"tier"`
	Role         string `json:"role"`
	Index        int    `json:"index"`
}

// ScenePromptShape is the simplified prompt projection: aspect plus whether
// the node carries inputs. Prompt text is never carried.
type ScenePromptShape struct {
	AspectRatio string `json:"aspect_ratio"`
	HasInputs   bool   `json:"has_inputs"`
}

// SceneNode is one projected card: normalized rect plus media identities.
type SceneNode struct {
	ID     string            `json:"id"`
	Kind   string            `json:"kind"`
	X      float64           `json:"x"`
	Y      float64           `json:"y"`
	Width  float64           `json:"width"`
	Height float64           `json:"height"`
	Aspect float64           `json:"aspect"`
	Media  []SceneMediaRef   `json:"media"`
	Blend  *ScenePoint       `json:"blend,omitempty"`
	Prompt *ScenePromptShape `json:"prompt,omitempty"`
}

// SceneConnection is one projected edge with resolved normalized endpoints.
// Connections referencing missing frames are dropped by extraction.
type SceneConnection struct {
	Key      string     `json:"key"`
	From     string     `json:"from"`
	To       string     `json:"to"`
	FromPort string     `json:"from_port"`
	ToPort   string     `json:"to_port"`
	Start    ScenePoint `json:"start"`
	End      ScenePoint `json:"end"`
}

// SceneGroup is one projected group: verbatim membership plus normalized
// rect. Empty groups are carried with IsEmpty; the renderer draws nothing.
type SceneGroup struct {
	ID      string   `json:"id"`
	Members []string `json:"members"`
	X       float64  `json:"x"`
	Y       float64  `json:"y"`
	Width   float64  `json:"width"`
	Height  float64  `json:"height"`
	IsEmpty bool     `json:"is_empty"`
}

// SceneBudgets carries the per-card enforcement limits. Nil means unset.
type SceneBudgets struct {
	MaxNodes       *int64 `json:"max_nodes"`
	MaxConnections *int64 `json:"max_connections"`
	MaxMediaRefs   *int64 `json:"max_media_refs"`
	MaxSceneBytes  *int64 `json:"max_scene_bytes"`
}

// SceneOmitted counts budget-truncated entries (over-limit semantics).
type SceneOmitted struct {
	Nodes        int `json:"nodes"`
	Connections  int `json:"connections"`
	MediaRefs    int `json:"media_refs"`
	Groups       int `json:"groups"`
	GroupMembers int `json:"group_members"`
}

// Scene is the versioned, bounded, pure-data board preview projection.
type Scene struct {
	Version        int               `json:"version"`
	ContentVersion string            `json:"content_version"`
	Bounds         *SceneBounds      `json:"bounds"`
	Extent         SceneExtent       `json:"extent"`
	Nodes          []SceneNode       `json:"nodes"`
	Connections    []SceneConnection `json:"connections"`
	Groups         []SceneGroup      `json:"groups"`
	Budgets        SceneBudgets      `json:"budgets"`
	Omitted        SceneOmitted      `json:"omitted"`
	Truncated      bool              `json:"truncated"`
}

// Limits carries optional enforcement caps for ApplyBudgets. Nil disables.
type Limits struct {
	MaxNodes       *int64
	MaxConnections *int64
	MaxMediaRefs   *int64
	MaxSceneBytes  *int64
}

// Snapshot subset: only scene-relevant fields are decoded. Unknown snapshot
// fields are ignored (the board schema owns them); required fields below
// are validated explicitly, never guessed.
type snapshotMediaRef struct {
	AssetID string `json:"assetId"`
	Tier    string `json:"derivativeTier"`
}

type snapshotOutput struct {
	URL          string            `json:"url"`
	Ratio        float64           `json:"ratio"`
	MediaRef     *snapshotMediaRef `json:"mediaRef"`
	MediaAssetID *int64            `json:"mediaAssetId"`
}

type snapshotInput struct {
	URL          string            `json:"url"`
	Ratio        float64           `json:"ratio"`
	MediaAssetID *int64            `json:"mediaAssetId"`
	MediaRef     *snapshotMediaRef `json:"mediaRef"`
}

type snapshotBlend struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type snapshotImageNode struct {
	ID             float64           `json:"id"`
	X              float64           `json:"x"`
	Y              float64           `json:"y"`
	Width          float64           `json:"width"`
	SourceImageURL string            `json:"sourceImageUrl"`
	SourceMediaRef *snapshotMediaRef `json:"sourceMediaRef"`
	SourceAsset    *int64            `json:"sourceMediaAssetId"`
	SourceRatio    float64           `json:"sourceImageRatio"`
	Outputs        []snapshotOutput  `json:"outputImages"`
}

type snapshotPromptNode struct {
	ID          float64         `json:"id"`
	X           float64         `json:"x"`
	Y           float64         `json:"y"`
	Width       float64         `json:"width"`
	AspectRatio string          `json:"aspectRatio"`
	Inputs      []snapshotInput `json:"inputImages"`
}

type snapshotFusionNode struct {
	ID      float64          `json:"id"`
	X       float64          `json:"x"`
	Y       float64          `json:"y"`
	Width   float64          `json:"width"`
	Blend   *snapshotBlend   `json:"blendPoint"`
	Inputs  []snapshotInput  `json:"inputImages"`
	Outputs []snapshotOutput `json:"outputImages"`
}

type snapshotConnection struct {
	ID       float64 `json:"id"`
	FromKind string  `json:"fromNodeType"`
	FromID   float64 `json:"fromNodeId"`
	FromPort string  `json:"fromPort"`
	ToKind   string  `json:"toNodeType"`
	ToID     float64 `json:"toNodeId"`
	ToPort   string  `json:"toPort"`
}

type snapshotGroup struct {
	ID      string   `json:"id"`
	Members []string `json:"nodeIds"`
	X       float64  `json:"x"`
	Y       float64  `json:"y"`
	Width   float64  `json:"width"`
	Height  float64  `json:"height"`
}

type snapshotDoc struct {
	SchemaVersion int                  `json:"schemaVersion"`
	ImageNodes    []snapshotImageNode  `json:"imageNodes"`
	PromptNodes   []snapshotPromptNode `json:"promptNodes"`
	FusionNodes   []snapshotFusionNode `json:"fusionNodes"`
	Connections   []snapshotConnection `json:"connections"`
	Groups        []snapshotGroup      `json:"groups"`
}

var sceneTiers = map[string]bool{"thumb": true, "preview": true, "large-preview": true}

var sceneAspects = map[string]bool{
	"auto": true, "1:1": true, "3:4": true, "4:3": true, "9:16": true, "16:9": true,
}

const (
	defaultImageWidth  = 280
	defaultPromptWidth = 280
	defaultFusionWidth = 286
	imageEmptySize     = 280
)

func isFinite(v float64) bool {
	return !math.IsNaN(v) && !math.IsInf(v, 0)
}

func integerID(v float64) (int64, bool) {
	if !isFinite(v) {
		return 0, false
	}
	id := int64(v)
	if float64(id) != v {
		return 0, false
	}
	return id, true
}

func sceneTierOf(ref *snapshotMediaRef) string {
	if ref != nil && sceneTiers[ref.Tier] {
		return ref.Tier
	}
	return DefaultSceneTier
}

func validAssetID(ref *snapshotMediaRef) string {
	if ref == nil {
		return ""
	}
	for _, r := range ref.AssetID {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return ""
		}
	}
	return ref.AssetID
}

func appendSceneMedia(out []SceneMediaRef, asset string, numeric *int64, tier, role string, index int) []SceneMediaRef {
	hasNumeric := numeric != nil && *numeric > 0
	if asset == "" && !hasNumeric {
		return out
	}
	var numCopy *int64
	if hasNumeric {
		v := *numeric
		numCopy = &v
	}
	return append(out, SceneMediaRef{
		Asset:        asset,
		AssetNumeric: numCopy,
		Tier:         tier,
		Role:         role,
		Index:        index,
	})
}

func usableOutput(o snapshotOutput) bool {
	return o.URL != "" || validAssetID(o.MediaRef) != "" || (o.MediaAssetID != nil && *o.MediaAssetID > 0)
}

// latestOutput mirrors the gallery priority: the newest usable output wins.
func latestOutput(outputs []snapshotOutput) (snapshotOutput, bool) {
	for i := len(outputs) - 1; i >= 0; i-- {
		if usableOutput(outputs[i]) {
			return outputs[i], true
		}
	}
	return snapshotOutput{}, false
}

func positiveRatio(v float64) (float64, bool) {
	if isFinite(v) && v > 0 {
		return v, true
	}
	return 1, false
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func nodeFrameID(kind string, id int64) string {
	return fmt.Sprintf("%s-%d", kind, id)
}

func frameWidth(raw float64, kind string) float64 {
	if isFinite(raw) && raw > 0 {
		return raw
	}
	switch kind {
	case "fusion":
		return defaultFusionWidth
	case "prompt":
		return defaultPromptWidth
	default:
		return defaultImageWidth
	}
}

type frameGeom struct {
	x, y, w, h float64
}

// Extract builds the Scene from an accepted persisted snapshot document
// (the payload object carrying schemaVersion plus node collections).
// contentVersion is the opaque content identity assigned by the acceptor;
// it is carried verbatim and never derived here.
func Extract(snapshot []byte, contentVersion string) (*Scene, error) {
	if contentVersion == "" {
		return nil, ErrEmptyContentVersion
	}
	var doc snapshotDoc
	if err := json.Unmarshal(snapshot, &doc); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidSnapshot, err)
	}
	if doc.SchemaVersion != SnapshotSchemaVersion {
		return nil, fmt.Errorf("%w: got %d", ErrUnsupportedSnapshot, doc.SchemaVersion)
	}
	nodes, frames := extractNodes(&doc)
	bounds, extent, scale, minX, minY := normalizeLayout(nodes, frames)
	return &Scene{
		Version:        SceneVersion,
		ContentVersion: contentVersion,
		Bounds:         bounds,
		Extent:         extent,
		Nodes:          nodes,
		Connections:    extractConnections(&doc, frames, scale, minX, minY),
		Groups:         extractGroups(&doc, scale, minX, minY),
		Budgets:        SceneBudgets{},
		Omitted:        SceneOmitted{},
		Truncated:      false,
	}, nil
}

// extractNodes projects nodes in stable collection order (image, prompt,
// fusion), preserving snapshot order within each collection. Rects are in
// board coordinates; normalizeLayout rewrites them in place.
func extractNodes(doc *snapshotDoc) ([]SceneNode, map[string]frameGeom) {
	nodes := []SceneNode{}
	frames := map[string]frameGeom{}
	for _, n := range doc.ImageNodes {
		id, ok := integerID(n.ID)
		if !ok || !isFinite(n.X) || !isFinite(n.Y) {
			continue
		}
		w := frameWidth(n.Width, "image")
		out, hasOut := latestOutput(n.Outputs)
		ratio, _ := positiveRatio(n.SourceRatio)
		hasPreview := hasOut
		if hasOut {
			if r, ok := positiveRatio(out.Ratio); ok {
				ratio = r
			}
		} else if n.SourceImageURL != "" || validAssetID(n.SourceMediaRef) != "" {
			hasPreview = true
		}
		h := float64(imageEmptySize)
		if hasPreview {
			h = math.Round(w * ratio)
			if h < 1 {
				h = 1
			}
		}
		media := []SceneMediaRef{}
		media = appendSceneMedia(media, validAssetID(n.SourceMediaRef), n.SourceAsset, sceneTierOf(n.SourceMediaRef), "source", 0)
		if hasOut {
			media = appendSceneMedia(media, validAssetID(out.MediaRef), out.MediaAssetID, sceneTierOf(out.MediaRef), "output", 0)
		}
		nodeID := nodeFrameID("image", id)
		nodes = append(nodes, SceneNode{
			ID: nodeID, Kind: "image",
			X: n.X, Y: n.Y, Width: w, Height: h, Aspect: w / h,
			Media: media,
		})
		frames[nodeID] = frameGeom{x: n.X, y: n.Y, w: w, h: h}
	}
	for _, n := range doc.PromptNodes {
		id, ok := integerID(n.ID)
		if !ok || !isFinite(n.X) || !isFinite(n.Y) {
			continue
		}
		w := frameWidth(n.Width, "prompt")
		media := []SceneMediaRef{}
		for i := range n.Inputs {
			media = appendSceneMedia(media, validAssetID(n.Inputs[i].MediaRef), n.Inputs[i].MediaAssetID, sceneTierOf(n.Inputs[i].MediaRef), "input", i)
		}
		aspect := n.AspectRatio
		if !sceneAspects[aspect] {
			aspect = "auto"
		}
		nodeID := nodeFrameID("prompt", id)
		nodes = append(nodes, SceneNode{
			ID: nodeID, Kind: "prompt",
			X: n.X, Y: n.Y, Width: w, Height: w, Aspect: 1,
			Media:  media,
			Prompt: &ScenePromptShape{AspectRatio: aspect, HasInputs: len(n.Inputs) > 0},
		})
		frames[nodeID] = frameGeom{x: n.X, y: n.Y, w: w, h: w}
	}
	for _, n := range doc.FusionNodes {
		id, ok := integerID(n.ID)
		if !ok || !isFinite(n.X) || !isFinite(n.Y) {
			continue
		}
		w := frameWidth(n.Width, "fusion")
		media := []SceneMediaRef{}
		for i := range n.Inputs {
			media = appendSceneMedia(media, validAssetID(n.Inputs[i].MediaRef), n.Inputs[i].MediaAssetID, sceneTierOf(n.Inputs[i].MediaRef), "input", i)
		}
		if out, ok := latestOutput(n.Outputs); ok {
			media = appendSceneMedia(media, validAssetID(out.MediaRef), out.MediaAssetID, sceneTierOf(out.MediaRef), "output", 0)
		}
		blend := &ScenePoint{X: 0.5, Y: 0.5}
		if n.Blend != nil && isFinite(n.Blend.X) && isFinite(n.Blend.Y) {
			blend = &ScenePoint{X: clamp01(n.Blend.X), Y: clamp01(n.Blend.Y)}
		}
		nodeID := nodeFrameID("fusion", id)
		nodes = append(nodes, SceneNode{
			ID: nodeID, Kind: "fusion",
			X: n.X, Y: n.Y, Width: w, Height: w, Aspect: 1,
			Media: media,
			Blend: blend,
		})
		frames[nodeID] = frameGeom{x: n.X, y: n.Y, w: w, h: w}
	}
	return nodes, frames
}

// normalizeLayout computes full-layout bounds over node frames only (group
// rects never stretch the layout), then rewrites node rects in place to
// normalized space via (v-min)*scale with scale = 1/max(width,height).
// The identical op order must be kept in every implementation so cross-
// language projections stay bit-identical.
func normalizeLayout(nodes []SceneNode, frames map[string]frameGeom) (*SceneBounds, SceneExtent, float64, float64, float64) {
	if len(frames) == 0 {
		return nil, SceneExtent{}, 1, 0, 0
	}
	minX := math.Inf(1)
	minY := math.Inf(1)
	maxX := math.Inf(-1)
	maxY := math.Inf(-1)
	for _, f := range frames {
		if f.x < minX {
			minX = f.x
		}
		if f.y < minY {
			minY = f.y
		}
		if f.x+f.w > maxX {
			maxX = f.x + f.w
		}
		if f.y+f.h > maxY {
			maxY = f.y + f.h
		}
	}
	width := maxX - minX
	height := maxY - minY
	span := width
	if height > span {
		span = height
	}
	if span <= 0 {
		return nil, SceneExtent{}, 1, 0, 0
	}
	scale := 1 / span
	bounds := &SceneBounds{MinX: minX, MinY: minY, MaxX: maxX, MaxY: maxY, Width: width, Height: height}
	extent := SceneExtent{Width: width * scale, Height: height * scale}
	for i := range nodes {
		f, ok := frames[nodes[i].ID]
		if !ok {
			continue
		}
		nodes[i].X = (f.x - minX) * scale
		nodes[i].Y = (f.y - minY) * scale
		nodes[i].Width = f.w * scale
		nodes[i].Height = f.h * scale
	}
	return bounds, extent, scale, minX, minY
}

// extractConnections resolves edges to normalized endpoints (right-edge
// center to left-edge center).
// Edges referencing missing frames are dropped.
func extractConnections(doc *snapshotDoc, frames map[string]frameGeom, scale, minX, minY float64) []SceneConnection {
	out := []SceneConnection{}
	for index, c := range doc.Connections {
		id, ok := integerID(c.ID)
		if !ok {
			continue
		}
		fromID, ok := integerID(c.FromID)
		if !ok {
			continue
		}
		toID, ok := integerID(c.ToID)
		if !ok {
			continue
		}
		fromKey := nodeFrameID(c.FromKind, fromID)
		toKey := nodeFrameID(c.ToKind, toID)
		from, ok := frames[fromKey]
		if !ok {
			continue
		}
		to, ok := frames[toKey]
		if !ok {
			continue
		}
		out = append(out, SceneConnection{
			Key:      fmt.Sprintf("connection-%d-%d", id, index),
			From:     fromKey,
			To:       toKey,
			FromPort: c.FromPort,
			ToPort:   c.ToPort,
			Start:    ScenePoint{X: (from.x + from.w - minX) * scale, Y: (from.y + from.h/2 - minY) * scale},
			End:      ScenePoint{X: (to.x - minX) * scale, Y: (to.y + to.h/2 - minY) * scale},
		})
	}
	return out
}

// extractGroups projects groups with verbatim membership (empty members are
// dropped; dangling references are kept verbatim for the renderer to skip).
// Rects use the same normalization as nodes and may exceed the unit range.
func extractGroups(doc *snapshotDoc, scale, minX, minY float64) []SceneGroup {
	out := []SceneGroup{}
	for _, g := range doc.Groups {
		if g.ID == "" {
			continue
		}
		members := []string{}
		for _, m := range g.Members {
			if m != "" {
				members = append(members, m)
			}
		}
		out = append(out, SceneGroup{
			ID:      g.ID,
			Members: members,
			X:       (g.X - minX) * scale,
			Y:       (g.Y - minY) * scale,
			Width:   g.Width * scale,
			Height:  g.Height * scale,
			IsEmpty: len(members) == 0,
		})
	}
	return out
}

// ApplyBudgets enforces limits deterministically in stable snapshot order.
// Dropped nodes take their dangling connections with them; media is capped
// across nodes in order. Nil limits disable truncation. Scenes that exceed
// MaxSceneBytes sheds trailing groups and their memberships before
// trailing nodes, using binary search. Enforced limits are recorded into
// scene.Budgets so the wire stays self-describing: null still means no
// truncation was applied.
func ApplyBudgets(scene *Scene, limits Limits) {
	if scene == nil {
		return
	}
	if limits.MaxNodes != nil {
		v := *limits.MaxNodes
		scene.Budgets.MaxNodes = &v
	}
	if limits.MaxConnections != nil {
		v := *limits.MaxConnections
		scene.Budgets.MaxConnections = &v
	}
	if limits.MaxMediaRefs != nil {
		v := *limits.MaxMediaRefs
		scene.Budgets.MaxMediaRefs = &v
	}
	if limits.MaxSceneBytes != nil {
		v := *limits.MaxSceneBytes
		scene.Budgets.MaxSceneBytes = &v
	}
	if limits.MaxNodes != nil && *limits.MaxNodes >= 0 && int64(len(scene.Nodes)) > *limits.MaxNodes {
		applyNodePrefix(scene, int(*limits.MaxNodes))
	}
	if limits.MaxConnections != nil && *limits.MaxConnections >= 0 && int64(len(scene.Connections)) > *limits.MaxConnections {
		keep := int(*limits.MaxConnections)
		scene.Omitted.Connections += len(scene.Connections) - keep
		scene.Connections = append([]SceneConnection{}, scene.Connections[:keep]...)
		scene.Truncated = true
	}
	if limits.MaxMediaRefs != nil && *limits.MaxMediaRefs >= 0 {
		remaining := int(*limits.MaxMediaRefs)
		for i := range scene.Nodes {
			if remaining <= 0 {
				scene.Omitted.MediaRefs += len(scene.Nodes[i].Media)
				scene.Nodes[i].Media = []SceneMediaRef{}
				continue
			}
			if len(scene.Nodes[i].Media) > remaining {
				scene.Omitted.MediaRefs += len(scene.Nodes[i].Media) - remaining
				scene.Nodes[i].Media = append([]SceneMediaRef{}, scene.Nodes[i].Media[:remaining]...)
				remaining = 0
			} else {
				remaining -= len(scene.Nodes[i].Media)
			}
		}
		if scene.Omitted.MediaRefs > 0 {
			scene.Truncated = true
		}
	}
	if limits.MaxSceneBytes != nil && *limits.MaxSceneBytes > 0 {
		shedBytesOverLimit(scene, *limits.MaxSceneBytes)
	}
}

// shedBytesOverLimit sheds group prefixes first, then node prefixes, preserving
// useful node geometry when large group membership dominates the payload.
func shedBytesOverLimit(scene *Scene, maxBytes int64) {
	if sceneFits(scene, maxBytes) {
		return
	}
	if len(scene.Groups) > 0 {
		low, high := 0, len(scene.Groups)
		for low < high {
			mid := (low + high + 1) / 2
			candidate := *scene
			applyGroupPrefix(&candidate, mid)
			if sceneFits(&candidate, maxBytes) {
				low = mid
			} else {
				high = mid - 1
			}
		}
		applyGroupPrefix(scene, low)
		if sceneFits(scene, maxBytes) {
			return
		}
	}
	low, high := 0, len(scene.Nodes)
	for low < high {
		mid := (low + high + 1) / 2
		if scenePrefixFits(scene, mid, maxBytes) {
			low = mid
		} else {
			high = mid - 1
		}
	}
	applyNodePrefix(scene, low)
}

func sceneFits(scene *Scene, maxBytes int64) bool {
	raw, err := json.Marshal(scene)
	return err == nil && int64(len(raw)) <= maxBytes
}

func applyGroupPrefix(scene *Scene, keep int) {
	if keep >= len(scene.Groups) {
		return
	}
	for _, group := range scene.Groups[keep:] {
		scene.Omitted.GroupMembers += len(group.Members)
	}
	scene.Omitted.Groups += len(scene.Groups) - keep
	scene.Groups = append([]SceneGroup{}, scene.Groups[:keep]...)
	scene.Truncated = true
}

// scenePrefixFits reports whether keeping the first keep nodes (plus
// surviving connections, with projected omission counts) fits maxBytes.
func scenePrefixFits(scene *Scene, keep int, maxBytes int64) bool {
	candidate := *scene
	applyNodePrefix(&candidate, keep)
	return sceneFits(&candidate, maxBytes)
}

// applyNodePrefix keeps the first keep nodes, removing connections that
// touch dropped nodes and counting each dropped node and connection
// exactly once.
func applyNodePrefix(scene *Scene, keep int) {
	if keep >= len(scene.Nodes) {
		return
	}
	dropped := make(map[string]bool, len(scene.Nodes)-keep)
	for _, n := range scene.Nodes[keep:] {
		dropped[n.ID] = true
		scene.Omitted.MediaRefs += len(n.Media)
	}
	scene.Omitted.Nodes += len(scene.Nodes) - keep
	scene.Nodes = append([]SceneNode{}, scene.Nodes[:keep]...)
	kept := make([]SceneConnection, 0, len(scene.Connections))
	for _, c := range scene.Connections {
		if dropped[c.From] || dropped[c.To] {
			scene.Omitted.Connections++
		} else {
			kept = append(kept, c)
		}
	}
	scene.Connections = kept
	scene.Truncated = true
}

// ParseScene strictly parses a scene payload: unknown fields are rejected,
// the version must equal SceneVersion, and semantic checks run fail-closed.
// It never guesses or fills defaults.
func ParseScene(data []byte) (*Scene, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var scene Scene
	if err := decoder.Decode(&scene); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidScene, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return nil, fmt.Errorf("%w: trailing payload", ErrInvalidScene)
	}
	if scene.Version != SceneVersion {
		return nil, fmt.Errorf("%w: got %d", ErrUnsupportedSceneVersion, scene.Version)
	}
	if err := checkScene(&scene); err != nil {
		return nil, err
	}
	return &scene, nil
}

func checkScene(scene *Scene) error {
	if scene.ContentVersion == "" {
		return fmt.Errorf("%w: content version is empty", ErrInvalidScene)
	}
	if scene.Bounds != nil {
		b := scene.Bounds
		for _, v := range []float64{b.MinX, b.MinY, b.MaxX, b.MaxY, b.Width, b.Height} {
			if !isFinite(v) {
				return fmt.Errorf("%w: non-finite bounds", ErrInvalidScene)
			}
		}
		if b.Width <= 0 || b.Height <= 0 {
			return fmt.Errorf("%w: degenerate bounds", ErrInvalidScene)
		}
	}
	for _, v := range []float64{scene.Extent.Width, scene.Extent.Height} {
		if !isFinite(v) || v < 0 {
			return fmt.Errorf("%w: invalid extent", ErrInvalidScene)
		}
	}
	seen := map[string]bool{}
	for _, n := range scene.Nodes {
		if n.ID == "" || n.Kind == "" {
			return fmt.Errorf("%w: node identity missing", ErrInvalidScene)
		}
		if seen[n.ID] {
			return fmt.Errorf("%w: duplicate node id", ErrInvalidScene)
		}
		seen[n.ID] = true
		if n.Kind != "image" && n.Kind != "prompt" && n.Kind != "fusion" {
			return fmt.Errorf("%w: unknown node kind", ErrInvalidScene)
		}
		for _, v := range []float64{n.X, n.Y, n.Width, n.Height, n.Aspect} {
			if !isFinite(v) {
				return fmt.Errorf("%w: non-finite node rect", ErrInvalidScene)
			}
		}
		if n.Width <= 0 || n.Height <= 0 || n.Aspect <= 0 {
			return fmt.Errorf("%w: degenerate node rect", ErrInvalidScene)
		}
		for _, m := range n.Media {
			if m.Asset == "" && (m.AssetNumeric == nil || *m.AssetNumeric <= 0) {
				return fmt.Errorf("%w: media without identity", ErrInvalidScene)
			}
			if m.Tier == "" || m.Role == "" {
				return fmt.Errorf("%w: media without tier or role", ErrInvalidScene)
			}
		}
		if n.Blend != nil && (!isFinite(n.Blend.X) || !isFinite(n.Blend.Y)) {
			return fmt.Errorf("%w: non-finite blend point", ErrInvalidScene)
		}
	}
	for _, c := range scene.Connections {
		if c.From == "" || c.To == "" || !seen[c.From] || !seen[c.To] {
			return fmt.Errorf("%w: dangling connection", ErrInvalidScene)
		}
		for _, v := range []float64{c.Start.X, c.Start.Y, c.End.X, c.End.Y} {
			if !isFinite(v) {
				return fmt.Errorf("%w: non-finite connection endpoint", ErrInvalidScene)
			}
		}
	}
	for _, g := range scene.Groups {
		if g.ID == "" {
			return fmt.Errorf("%w: group without id", ErrInvalidScene)
		}
		for _, v := range []float64{g.X, g.Y, g.Width, g.Height} {
			if !isFinite(v) {
				return fmt.Errorf("%w: non-finite group rect", ErrInvalidScene)
			}
		}
	}
	return nil
}
