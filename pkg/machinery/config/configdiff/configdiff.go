// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

// Package configdiff provides a way to compare two config trees.
package configdiff

import (
	"strings"

	"github.com/siderolabs/talos/pkg/machinery/config"
	configdoc "github.com/siderolabs/talos/pkg/machinery/config/config"
	"github.com/siderolabs/talos/pkg/machinery/config/encoder"
	"github.com/siderolabs/talos/pkg/machinery/config/internal/documentid"
	"github.com/siderolabs/talos/pkg/machinery/textdiff"
)

// DiffConfigs returns a string representation of the diff between two machine configurations.
//
// One of the resources might be nil.
func DiffConfigs(oldCfg, newCfg config.Encoder) (string, error) {
	oldDocuments, oldDocumentsOK := documentsForDiff(oldCfg)
	newDocuments, newDocumentsOK := documentsForDiff(newCfg)

	if oldDocumentsOK && newDocumentsOK {
		oldDocuments, newDocuments = alignDocuments(oldDocuments, newDocuments)

		oldYAML, err := encodeDocuments(oldDocuments)
		if err != nil {
			return "", err
		}

		newYAML, err := encodeDocuments(newDocuments)
		if err != nil {
			return "", err
		}

		return textdiff.Diff(oldYAML, newYAML)
	}

	var oldYAML, newYAML []byte

	var err error

	if oldCfg != nil {
		oldYAML, err = oldCfg.EncodeBytes(encoder.WithComments(encoder.CommentsDisabled))
		if err != nil {
			return "", err
		}
	}

	if newCfg != nil {
		newYAML, err = newCfg.EncodeBytes(encoder.WithComments(encoder.CommentsDisabled))
		if err != nil {
			return "", err
		}
	}

	return textdiff.Diff(string(oldYAML), string(newYAML))
}

type documentEncoder interface {
	Documents() []configdoc.Document
}

func documentsForDiff(cfg config.Encoder) ([]configdoc.Document, bool) {
	if cfg == nil {
		return nil, true
	}

	documents, ok := cfg.(documentEncoder)
	if !ok {
		return nil, false
	}

	return documents.Documents(), true
}

// alignDocuments puts matching documents into the same order before running the
// text diff. Document order has no semantic meaning, so changes in that order
// should not make an otherwise unchanged document appear removed and re-added.
func alignDocuments(oldDocuments, newDocuments []configdoc.Document) ([]configdoc.Document, []configdoc.Document) {
	oldByID := make(map[documentid.DocumentID]configdoc.Document, len(oldDocuments))
	newByID := make(map[documentid.DocumentID]configdoc.Document, len(newDocuments))

	for _, document := range oldDocuments {
		oldByID[documentid.Extract(document)] = document
	}

	for _, document := range newDocuments {
		newByID[documentid.Extract(document)] = document
	}

	// Keep removed documents close to their position in the old configuration:
	// place them immediately before the next document which still exists.
	removedBefore := make(map[documentid.DocumentID][]documentid.DocumentID)
	pendingRemoved := make([]documentid.DocumentID, 0)

	for _, document := range oldDocuments {
		id := documentid.Extract(document)
		if _, exists := newByID[id]; !exists {
			pendingRemoved = append(pendingRemoved, id)

			continue
		}

		removedBefore[id] = append(removedBefore[id], pendingRemoved...)
		pendingRemoved = pendingRemoved[:0]
	}

	order := make([]documentid.DocumentID, 0, len(oldDocuments)+len(newDocuments))

	for _, document := range newDocuments {
		id := documentid.Extract(document)
		order = append(order, removedBefore[id]...)
		order = append(order, id)
	}

	order = append(order, pendingRemoved...)

	alignedOld := make([]configdoc.Document, 0, len(oldDocuments))
	alignedNew := make([]configdoc.Document, 0, len(newDocuments))

	for _, id := range order {
		if document, exists := oldByID[id]; exists {
			alignedOld = append(alignedOld, document)
		}

		if document, exists := newByID[id]; exists {
			alignedNew = append(alignedNew, document)
		}
	}

	return alignedOld, alignedNew
}

func encodeDocuments(documents []configdoc.Document) (string, error) {
	var result strings.Builder

	for i, document := range documents {
		if i > 0 {
			result.WriteString("---\n")
		}

		encoded, err := encoder.NewEncoder(document, encoder.WithComments(encoder.CommentsDisabled)).Encode()
		if err != nil {
			return "", err
		}

		result.Write(encoded)
	}

	return result.String(), nil
}
