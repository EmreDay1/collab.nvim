package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

type OperationType string

const (
	OpInsert OperationType = "insert"
	OpDelete OperationType = "delete"
	OpRetain OperationType = "retain"
)

type Operation struct {
	Type      OperationType `json:"type"`
	Position  int           `json:"position"`
	Content   string        `json:"content"`
	Length    int           `json:"length"`
	UserID    string        `json:"user_id"`
	Timestamp int64         `json:"timestamp"`
	ID        string        `json:"id"`
	VectorClock VectorClock `json:"vector_clock"`
}

type VectorClock map[string]int64

func (vc VectorClock) Copy() VectorClock {
	copy := make(VectorClock)
	for k, v := range vc {
		copy[k] = v
	}
	return copy
}

func (vc VectorClock) Increment(userID string) {
	vc[userID]++
}

func (vc VectorClock) Update(other VectorClock) {
	for userID, timestamp := range other {
		if vc[userID] < timestamp {
			vc[userID] = timestamp
		}
	}
}

func (vc VectorClock) HappensBefore(other VectorClock) bool {
	hasSmaller := false
	for userID, timestamp := range vc {
		otherTimestamp, exists := other[userID]
		if !exists {
			otherTimestamp = 0
		}
		if timestamp > otherTimestamp {
			return false
		}
		if timestamp < otherTimestamp {
			hasSmaller = true
		}
	}
	
	for userID, otherTimestamp := range other {
		timestamp, exists := vc[userID]
		if !exists {
			timestamp = 0
		}
		if timestamp > otherTimestamp {
			return false
		}
		if timestamp < otherTimestamp {
			hasSmaller = true
		}
	}
	
	return hasSmaller
}

func (vc VectorClock) IsConcurrent(other VectorClock) bool {
	return !vc.HappensBefore(other) && !other.HappensBefore(vc) && !vc.Equals(other)
}

func (vc VectorClock) Equals(other VectorClock) bool {
	if len(vc) != len(other) {
		// Check if missing entries are all zero
		allUsers := make(map[string]bool)
		for userID := range vc {
			allUsers[userID] = true
		}
		for userID := range other {
			allUsers[userID] = true
		}
		
		for userID := range allUsers {
			vcTime, vcExists := vc[userID]
			otherTime, otherExists := other[userID]
			
			if !vcExists {
				vcTime = 0
			}
			if !otherExists {
				otherTime = 0
			}
			
			if vcTime != otherTime {
				return false
			}
		}
		return true
	}
	
	for userID, timestamp := range vc {
		if other[userID] != timestamp {
			return false
		}
	}
	return true
}

type DocumentState struct {
	Content     string                `json:"content"`
	Version     int64                 `json:"version"`
	Operations  []Operation          `json:"operations"`
	VectorClock VectorClock          `json:"vector_clock"`
	mutex       sync.RWMutex
}

type OperationBuffer struct {
	operations []Operation
	mutex      sync.RWMutex
}

func (ob *OperationBuffer) Add(op Operation) {
	ob.mutex.Lock()
	defer ob.mutex.Unlock()
	ob.operations = append(ob.operations, op)
}

func (ob *OperationBuffer) GetAll() []Operation {
	ob.mutex.RLock()
	defer ob.mutex.RUnlock()
	result := make([]Operation, len(ob.operations))
	copy(result, ob.operations)
	return result
}

func (ob *OperationBuffer) Clear() {
	ob.mutex.Lock()
	defer ob.mutex.Unlock()
	ob.operations = make([]Operation, 0)
}

func (ob *OperationBuffer) RemoveApplied(appliedOps []Operation) {
	ob.mutex.Lock()
	defer ob.mutex.Unlock()
	
	appliedSet := make(map[string]bool)
	for _, op := range appliedOps {
		appliedSet[op.ID] = true
	}
	
	filtered := make([]Operation, 0)
	for _, op := range ob.operations {
		if !appliedSet[op.ID] {
			filtered = append(filtered, op)
		}
	}
	ob.operations = filtered
}

type SyncManager struct {
	document          *DocumentState
	userID            string
	vectorClock       VectorClock
	
	// Operation buffers
	localBuffer       *OperationBuffer
	remoteBuffer      *OperationBuffer
	acknowledgedOps   map[string]bool
	
	// Synchronization state
	isTransforming    bool
	transformMutex    sync.RWMutex
	
	// Event handlers
	onDocumentChanged  func(content string)
	onOperationApplied func(op Operation)
	onConflictResolved func(localOp, remoteOp Operation, resolution Operation)
	
	// Advanced OT state
	stateVector       map[string]int64  // State vector for each peer
	operationHistory  []Operation       // Complete operation history
	maxHistorySize    int              // Maximum history size before cleanup
}

func NewSyncManager() *SyncManager {
	return &SyncManager{
		document: &DocumentState{
			Content:     "",
			Version:     0,
			Operations:  make([]Operation, 0),
			VectorClock: make(VectorClock),
		},
		vectorClock:      make(VectorClock),
		localBuffer:      &OperationBuffer{operations: make([]Operation, 0)},
		remoteBuffer:     &OperationBuffer{operations: make([]Operation, 0)},
		acknowledgedOps:  make(map[string]bool),
		stateVector:      make(map[string]int64),
		operationHistory: make([]Operation, 0),
		maxHistorySize:   1000,
	}
}

func (sm *SyncManager) SetUserID(userID string) {
	sm.userID = userID
	sm.vectorClock[userID] = 0
	sm.stateVector[userID] = 0
}

func (sm *SyncManager) SetEventHandlers(
	onDocumentChanged func(string),
	onOperationApplied func(Operation),
	onConflictResolved func(Operation, Operation, Operation),
) {
	sm.onDocumentChanged = onDocumentChanged
	sm.onOperationApplied = onOperationApplied
	sm.onConflictResolved = onConflictResolved
}

func (sm *SyncManager) InitializeDocument(content string) {
	sm.document.mutex.Lock()
	defer sm.document.mutex.Unlock()
	
	sm.document.Content = content
	sm.document.Version = 0
	sm.document.Operations = make([]Operation, 0)
	sm.document.VectorClock = make(VectorClock)
	sm.vectorClock = make(VectorClock)
	sm.vectorClock[sm.userID] = 0
}

func (sm *SyncManager) GetDocumentContent() string {
	sm.document.mutex.RLock()
	defer sm.document.mutex.RUnlock()
	return sm.document.Content
}

func (sm *SyncManager) GetDocumentVersion() int64 {
	sm.document.mutex.RLock()
	defer sm.document.mutex.RUnlock()
	return sm.document.Version
}

func (sm *SyncManager) GetVectorClock() VectorClock {
	return sm.vectorClock.Copy()
}

func (sm *SyncManager) CreateInsertOperation(position int, content string) Operation {
	sm.vectorClock.Increment(sm.userID)
	
	return Operation{
		Type:        OpInsert,
		Position:    position,
		Content:     content,
		Length:      len(content),
		UserID:      sm.userID,
		Timestamp:   time.Now().UnixNano(),
		ID:          generateOperationID(sm.userID),
		VectorClock: sm.vectorClock.Copy(),
	}
}

func (sm *SyncManager) CreateDeleteOperation(position int, length int) Operation {
	sm.vectorClock.Increment(sm.userID)
	
	// Extract the content being deleted for better conflict resolution
	content := ""
	sm.document.mutex.RLock()
	if position >= 0 && position < len(sm.document.Content) {
		endPos := position + length
		if endPos > len(sm.document.Content) {
			endPos = len(sm.document.Content)
		}
		content = sm.document.Content[position:endPos]
	}
	sm.document.mutex.RUnlock()
	
	return Operation{
		Type:        OpDelete,
		Position:    position,
		Content:     content, // Store deleted content for OT
		Length:      length,
		UserID:      sm.userID,
		Timestamp:   time.Now().UnixNano(),
		ID:          generateOperationID(sm.userID),
		VectorClock: sm.vectorClock.Copy(),
	}
}

func (sm *SyncManager) ApplyLocalOperation(op Operation) error {
	// Add to local buffer
	sm.localBuffer.Add(op)
	
	// Apply to document immediately (optimistic execution)
	err := sm.applyOperationToDocument(op)
	if err != nil {
		return fmt.Errorf("failed to apply local operation: %v", err)
	}
	
	// Update our vector clock
	sm.vectorClock.Update(op.VectorClock)
	
	// Add to operation history
	sm.addToHistory(op)
	
	return nil
}

func (sm *SyncManager) ApplyRemoteOperation(remoteOp Operation) error {
	sm.transformMutex.Lock()
	defer sm.transformMutex.Unlock()
	
	// Add to remote buffer
	sm.remoteBuffer.Add(remoteOp)
	
	// Update vector clock
	sm.vectorClock.Update(remoteOp.VectorClock)
	
	// Get all operations that need transformation
	localOps := sm.localBuffer.GetAll()
	
	// Perform operational transformation
	transformedOp, transformedLocalOps, err := sm.performOperationalTransformation(remoteOp, localOps)
	if err != nil {
		return fmt.Errorf("operational transformation failed: %v", err)
	}
	
	// Undo local operations (we need to reapply them after transformation)
	err = sm.undoLocalOperations(localOps)
	if err != nil {
		return fmt.Errorf("failed to undo local operations: %v", err)
	}
	
	// Apply transformed remote operation
	err = sm.applyOperationToDocument(transformedOp)
	if err != nil {
		return fmt.Errorf("failed to apply transformed remote operation: %v", err)
	}
	
	// Reapply transformed local operations
	for _, transformedLocalOp := range transformedLocalOps {
		err = sm.applyOperationToDocument(transformedLocalOp)
		if err != nil {
			return fmt.Errorf("failed to reapply transformed local operation: %v", err)
		}
	}
	
	// Update local buffer with transformed operations
	sm.localBuffer.Clear()
	for _, op := range transformedLocalOps {
		sm.localBuffer.Add(op)
	}
	
	// Add to operation history
	sm.addToHistory(transformedOp)
	
	// Notify about operation
	if sm.onOperationApplied != nil {
		sm.onOperationApplied(transformedOp)
	}
	
	return nil
}

func (sm *SyncManager) performOperationalTransformation(remoteOp Operation, localOps []Operation) (Operation, []Operation, error) {
	transformedRemoteOp := remoteOp
	transformedLocalOps := make([]Operation, len(localOps))
	copy(transformedLocalOps, localOps)
	
	// Sort operations by vector clock causality
	allOps := append([]Operation{remoteOp}, localOps...)
	sortedOps := sm.topologicalSort(allOps)
	
	// Apply inclusion transformation (IT)
	for i, op1 := range sortedOps {
		for j := i + 1; j < len(sortedOps); j++ {
			op2 := sortedOps[j]
			
			// Determine transformation direction based on causality
			if op1.VectorClock.HappensBefore(op2.VectorClock) {
				// op1 happened before op2, transform op2 against op1
				if op2.ID == remoteOp.ID {
					transformedRemoteOp = sm.inclusionTransform(transformedRemoteOp, op1, false)
				} else {
					// Find and update in transformedLocalOps
					for k, localOp := range transformedLocalOps {
						if localOp.ID == op2.ID {
							transformedLocalOps[k] = sm.inclusionTransform(localOp, op1, false)
							break
						}
					}
				}
			} else if op2.VectorClock.HappensBefore(op1.VectorClock) {
				// op2 happened before op1, transform op1 against op2
				if op1.ID == remoteOp.ID {
					transformedRemoteOp = sm.inclusionTransform(transformedRemoteOp, op2, true)
				} else {
					// Find and update in transformedLocalOps
					for k, localOp := range transformedLocalOps {
						if localOp.ID == op1.ID {
							transformedLocalOps[k] = sm.inclusionTransform(localOp, op2, true)
							break
						}
					}
				}
			} else if op1.VectorClock.IsConcurrent(op2.VectorClock) {
				// Concurrent operations - use deterministic tiebreaker
				priority1 := sm.calculatePriority(op1)
				priority2 := sm.calculatePriority(op2)
				
				if priority1 < priority2 {
					// op1 has higher priority
					if op2.ID == remoteOp.ID {
						transformedRemoteOp = sm.inclusionTransform(transformedRemoteOp, op1, false)
					} else {
						for k, localOp := range transformedLocalOps {
							if localOp.ID == op2.ID {
								transformedLocalOps[k] = sm.inclusionTransform(localOp, op1, false)
								break
							}
						}
					}
				} else {
					// op2 has higher priority
					if op1.ID == remoteOp.ID {
						transformedRemoteOp = sm.inclusionTransform(transformedRemoteOp, op2, true)
					} else {
						for k, localOp := range transformedLocalOps {
							if localOp.ID == op1.ID {
								transformedLocalOps[k] = sm.inclusionTransform(localOp, op2, true)
								break
							}
						}
					}
				}
				
				// Notify about conflict resolution
				if sm.onConflictResolved != nil {
					if op1.ID == remoteOp.ID {
						sm.onConflictResolved(op2, op1, transformedRemoteOp)
					} else {
						sm.onConflictResolved(op1, op2, transformedLocalOps[0]) // Simplified
					}
				}
			}
		}
	}
	
	return transformedRemoteOp, transformedLocalOps, nil
}

func (sm *SyncManager) inclusionTransform(op1, op2 Operation, op1HasPriority bool) Operation {
	result := op1
	
	switch {
	case op1.Type == OpInsert && op2.Type == OpInsert:
		result = sm.transformInsertInsert(op1, op2, op1HasPriority)
	case op1.Type == OpInsert && op2.Type == OpDelete:
		result = sm.transformInsertDelete(op1, op2)
	case op1.Type == OpDelete && op2.Type == OpInsert:
		result = sm.transformDeleteInsert(op1, op2)
	case op1.Type == OpDelete && op2.Type == OpDelete:
		result = sm.transformDeleteDelete(op1, op2, op1HasPriority)
	}
	
	return result
}

func (sm *SyncManager) transformInsertInsert(op1, op2 Operation, op1HasPriority bool) Operation {
	if op2.Position < op1.Position {
		// op2 is before op1, shift op1 right
		return Operation{
			Type:        op1.Type,
			Position:    op1.Position + op2.Length,
			Content:     op1.Content,
			Length:      op1.Length,
			UserID:      op1.UserID,
			Timestamp:   op1.Timestamp,
			ID:          op1.ID,
			VectorClock: op1.VectorClock,
		}
	} else if op2.Position == op1.Position {
		// Same position - use priority for deterministic ordering
		if op1HasPriority {
			return op1 // op1 stays at same position
		} else {
			// op2 has priority, shift op1 right
			return Operation{
				Type:        op1.Type,
				Position:    op1.Position + op2.Length,
				Content:     op1.Content,
				Length:      op1.Length,
				UserID:      op1.UserID,
				Timestamp:   op1.Timestamp,
				ID:          op1.ID,
				VectorClock: op1.VectorClock,
			}
		}
	}
	
	// op2 is after op1, no transformation needed
	return op1
}

func (sm *SyncManager) transformInsertDelete(op1, op2 Operation) Operation {
	if op2.Position <= op1.Position {
		// Delete is before or at insert position
		if op2.Position + op2.Length <= op1.Position {
			// Delete is completely before insert, shift insert left
			return Operation{
				Type:        op1.Type,
				Position:    op1.Position - op2.Length,
				Content:     op1.Content,
				Length:      op1.Length,
				UserID:      op1.UserID,
				Timestamp:   op1.Timestamp,
				ID:          op1.ID,
				VectorClock: op1.VectorClock,
			}
		} else {
			// Delete overlaps with insert position, place insert at delete start
			return Operation{
				Type:        op1.Type,
				Position:    op2.Position,
				Content:     op1.Content,
				Length:      op1.Length,
				UserID:      op1.UserID,
				Timestamp:   op1.Timestamp,
				ID:          op1.ID,
				VectorClock: op1.VectorClock,
			}
		}
	}
	
	// Delete is after insert, no transformation needed
	return op1
}

func (sm *SyncManager) transformDeleteInsert(op1, op2 Operation) Operation {
	if op2.Position <= op1.Position {
		// Insert is before delete, shift delete right
		return Operation{
			Type:        op1.Type,
			Position:    op1.Position + op2.Length,
			Content:     op1.Content,
			Length:      op1.Length,
			UserID:      op1.UserID,
			Timestamp:   op1.Timestamp,
			ID:          op1.ID,
			VectorClock: op1.VectorClock,
		}
	} else if op2.Position < op1.Position + op1.Length {
		// Insert is within delete range, adjust delete length
		return Operation{
			Type:        op1.Type,
			Position:    op1.Position,
			Content:     op1.Content,
			Length:      op1.Length + op2.Length,
			UserID:      op1.UserID,
			Timestamp:   op1.Timestamp,
			ID:          op1.ID,
			VectorClock: op1.VectorClock,
		}
	}
	
	// Insert is after delete, no transformation needed
	return op1
}

func (sm *SyncManager) transformDeleteDelete(op1, op2 Operation, op1HasPriority bool) Operation {
	if op2.Position + op2.Length <= op1.Position {
		// op2 is completely before op1, shift op1 left
		return Operation{
			Type:        op1.Type,
			Position:    op1.Position - op2.Length,
			Content:     op1.Content,
			Length:      op1.Length,
			UserID:      op1.UserID,
			Timestamp:   op1.Timestamp,
			ID:          op1.ID,
			VectorClock: op1.VectorClock,
		}
	} else if op1.Position + op1.Length <= op2.Position {
		// op1 is completely before op2, no transformation needed
		return op1
	} else {
		// Overlapping deletes - complex case
		start1, end1 := op1.Position, op1.Position + op1.Length
		start2, end2 := op2.Position, op2.Position + op2.Length
		
		if start2 <= start1 && end2 >= end1 {
			// op2 completely covers op1, op1 becomes empty
			return Operation{
				Type:        op1.Type,
				Position:    start2,
				Content:     "",
				Length:      0,
				UserID:      op1.UserID,
				Timestamp:   op1.Timestamp,
				ID:          op1.ID,
				VectorClock: op1.VectorClock,
			}
		} else if start1 <= start2 && end1 >= end2 {
			// op1 completely covers op2, adjust op1 length
			return Operation{
				Type:        op1.Type,
				Position:    op1.Position,
				Content:     op1.Content,
				Length:      op1.Length - op2.Length,
				UserID:      op1.UserID,
				Timestamp:   op1.Timestamp,
				ID:          op1.ID,
				VectorClock: op1.VectorClock,
			}
		} else {
			// Partial overlap - determine resolution based on priority and positions
			newStart := start1
			newLength := op1.Length
			
			if start2 < start1 {
				// op2 starts before op1
				overlap := end2 - start1
				newStart = start2
				newLength = op1.Length - overlap
			} else {
				// op1 starts before op2
				overlap := end1 - start2
				newLength = op1.Length - overlap
			}
			
			if newLength < 0 {
				newLength = 0
			}
			
			return Operation{
				Type:        op1.Type,
				Position:    newStart,
				Content:     op1.Content,
				Length:      newLength,
				UserID:      op1.UserID,
				Timestamp:   op1.Timestamp,
				ID:          op1.ID,
				VectorClock: op1.VectorClock,
			}
		}
	}
}

func (sm *SyncManager) calculatePriority(op Operation) int64 {
	// Use a combination of user ID hash and timestamp for deterministic priority
	hash := hashString(op.UserID + op.ID)
	return hash + op.Timestamp
}

func (sm *SyncManager) topologicalSort(operations []Operation) []Operation {
	// Sort operations based on causality (vector clocks)
	sorted := make([]Operation, len(operations))
	copy(sorted, operations)
	
	sort.Slice(sorted, func(i, j int) bool {
		op1, op2 := sorted[i], sorted[j]
		
		if op1.VectorClock.HappensBefore(op2.VectorClock) {
			return true
		}
		if op2.VectorClock.HappensBefore(op1.VectorClock) {
			return false
		}
		
		// Concurrent operations - sort by priority
		return sm.calculatePriority(op1) < sm.calculatePriority(op2)
	})
	
	return sorted
}

func (sm *SyncManager) applyOperationToDocument(op Operation) error {
	sm.document.mutex.Lock()
	defer sm.document.mutex.Unlock()
	
	content := sm.document.Content
	
	switch op.Type {
	case OpInsert:
		if op.Position < 0 || op.Position > len(content) {
			return fmt.Errorf("invalid insert position %d for document length %d", op.Position, len(content))
		}
		
		newContent := content[:op.Position] + op.Content + content[op.Position:]
		sm.document.Content = newContent
		
	case OpDelete:
		if op.Position < 0 || op.Position >= len(content) {
			// Position is invalid, but this might be due to concurrent operations
			// Skip this operation rather than error
			return nil
		}
		
		endPos := op.Position + op.Length
		if endPos > len(content) {
			endPos = len(content)
		}
		
		if endPos <= op.Position {
			// Nothing to delete
			return nil
		}
		
		newContent := content[:op.Position] + content[endPos:]
		sm.document.Content = newContent
		
	default:
		return fmt.Errorf("unknown operation type: %s", op.Type)
	}
	
	// Update document state
	sm.document.Version++
	sm.document.VectorClock.Update(op.VectorClock)
	sm.document.Operations = append(sm.document.Operations, op)
	
	// Notify about document change
	if sm.onDocumentChanged != nil {
		sm.onDocumentChanged(sm.document.Content)
	}
	
	return nil
}

func (sm *SyncManager) undoLocalOperations(operations []Operation) error {
	// Reconstruct document state without local operations
	// This is a simplified approach - in practice, you might want to use snapshots
	
	sm.document.mutex.Lock()
	defer sm.document.mutex.Unlock()
	
	// Get all operations except the ones we're undoing
	localOpIDs := make(map[string]bool)
	for _, op := range operations {
		localOpIDs[op.ID] = true
	}
	
	// Rebuild document from remaining operations
	sm.document.Content = ""
	sm.document.Version = 0
	remainingOps := make([]Operation, 0)
	
	for _, op := range sm.document.Operations {
		if !localOpIDs[op.ID] {
			remainingOps = append(remainingOps, op)
		}
	}
	
	sm.document.Operations = make([]Operation, 0)
	
	// Reapply remaining operations
	for _, op := range remainingOps {
		err := sm.applyOperationDirectly(op)
		if err != nil {
			return fmt.Errorf("failed to reapply operation during undo: %v", err)
		}
	}
	
	return nil
}

func (sm *SyncManager) applyOperationDirectly(op Operation) error {
	// Apply operation without mutex (assumes caller holds lock)
	content := sm.document.Content
	
	switch op.Type {
	case OpInsert:
		if op.Position >= 0 && op.Position <= len(content) {
			sm.document.Content = content[:op.Position] + op.Content + content[op.Position:]
		}
	case OpDelete:
		if op.Position >= 0 && op.Position < len(content) {
			endPos := op.Position + op.Length
			if endPos > len(content) {
				endPos = len(content)
			}
			if endPos > op.Position {
				sm.document.Content = content[:op.Position] + content[endPos:]
			}
		}
	}
	
	sm.document.Version++
	sm.document.VectorClock.Update(op.VectorClock)
	sm.document.Operations = append(sm.document.Operations, op)
	
	return nil
}

func (sm *SyncManager) addToHistory(op Operation) {
	if len(sm.operationHistory) >= sm.maxHistorySize {
		// Remove oldest operations
		sm.operationHistory = sm.operationHistory[len(sm.operationHistory)/2:]
	}
	sm.operationHistory = append(sm.operationHistory, op)
}

func (sm *SyncManager) GetOperationsSince(vectorClock VectorClock) []Operation {
	sm.document.mutex.RLock()
	defer sm.document.mutex.RUnlock()
	
	var operations []Operation
	for _, op := range sm.document.Operations {
		if !op.VectorClock.HappensBefore(vectorClock) && !op.VectorClock.Equals(vectorClock) {
			operations = append(operations, op)
		}
	}
	
	return operations
}

func (sm *SyncManager) SerializeOperation(op Operation) ([]byte, error) {
	return json.Marshal(op)
}

func (sm *SyncManager) DeserializeOperation(data []byte) (Operation, error) {
	var op Operation
	err := json.Unmarshal(data, &op)
	return op, err
}

func (sm *SyncManager) GetDocumentState() DocumentState {
	sm.document.mutex.RLock()
	defer sm.document.mutex.RUnlock()
	
	return DocumentState{
		Content:     sm.document.Content,
		Version:     sm.document.Version,
		Operations:  append([]Operation(nil), sm.document.Operations...),
		VectorClock: sm.document.VectorClock.Copy(),
	}
}

func (sm *SyncManager) AcknowledgeOperation(opID string) {
	sm.acknowledgedOps[opID] = true
}

func (sm *SyncManager) CleanupHistory() {
	// Remove acknowledged operations from buffers
	localOps := sm.localBuffer.GetAll()
	acknowledgedLocal := make([]Operation, 0)
	for _, op := range localOps {
		if sm.acknowledgedOps[op.ID] {
			acknowledgedLocal = append(acknowledgedLocal, op)
		}
	}
	sm.localBuffer.RemoveApplied(acknowledgedLocal)
	
	// Clean up acknowledgment map
	for opID := range sm.acknowledgedOps {
		found := false
		for _, op := range localOps {
			if op.ID == opID {
				found = true
				break
			}
		}
		if !found {
			delete(sm.acknowledgedOps, opID)
		}
	}
}

// Utility functions
func generateOperationID(userID string) string {
	bytes := make([]byte, 8)
	rand.Read(bytes)
	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("%s-%d-%s", userID, timestamp, hex.EncodeToString(bytes))
}

func hashString(s string) int64 {
	var hash int64 = 5381
	for _, c := range s {
		hash = ((hash << 5) + hash) + int64(c)
	}
	if hash < 0 {
		hash = -hash
	}
	return hash
}
