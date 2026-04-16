// internal/resource/plan.go
package resource

// Plan computes the diff between actual and desired resource maps.
// Returns topologically-sorted phases of events.
// Creates are ordered so dependencies come first.
// Deletes are ordered so dependents are deleted before their dependencies.
// Recreates (NeedsRecreate) produce a Delete phase followed by a Create phase.
func Plan(actual, desired map[Identifier]Resource) [][]Event {
	var deletes []Event
	var creates []Event
	var updates []Event

	// Resources in actual but not desired → delete
	for id, r := range actual {
		if _, want := desired[id]; !want {
			deletes = append(deletes, Event{Action: ActionDelete, Resource: r})
		}
	}

	// Resources that need recreation, update, or creation
	for id, r := range desired {
		if _, have := actual[id]; have {
			s := r.Status()
			if s.Exists && s.NeedsRecreate {
				deletes = append(deletes, Event{Action: ActionDelete, Resource: r})
				creates = append(creates, Event{Action: ActionCreate, Resource: r})
			} else if s.Exists && s.NeedsUpdate {
				updates = append(updates, Event{Action: ActionUpdate, Resource: r})
			}
			// Exists and healthy → no-op
			continue
		}
		// Not in actual → create
		creates = append(creates, Event{Action: ActionCreate, Resource: r})
	}

	var phases [][]Event

	// Delete phases (reverse dependency order — dependents first)
	if len(deletes) > 0 {
		deletePhases := topoSort(deletes, true)
		phases = append(phases, deletePhases...)
	}

	// Update phases (dependency order — dependencies first)
	if len(updates) > 0 {
		updatePhases := topoSort(updates, false)
		phases = append(phases, updatePhases...)
	}

	// Create phases (dependency order — dependencies first)
	if len(creates) > 0 {
		createPhases := topoSort(creates, false)
		phases = append(phases, createPhases...)
	}

	return phases
}

// topoSort orders events into phases respecting dependencies.
// If reverse=true, dependents come before dependencies (for deletes).
func topoSort(events []Event, reverse bool) [][]Event {
	eventSet := make(map[Identifier]Event)
	for _, e := range events {
		eventSet[e.Resource.Identifier()] = e
	}

	// Build in-degree map (only for events in our set)
	inDegree := make(map[Identifier]int)
	dependents := make(map[Identifier][]Identifier)

	for _, e := range events {
		id := e.Resource.Identifier()
		inDegree[id] = 0
	}

	for _, e := range events {
		id := e.Resource.Identifier()
		for _, dep := range e.Resource.Dependencies() {
			if _, inSet := eventSet[dep]; inSet {
				if reverse {
					// For deletes: dependent must go before dependency
					inDegree[dep]++
					dependents[id] = append(dependents[id], dep)
				} else {
					// For creates: dependency must go before dependent
					inDegree[id]++
					dependents[dep] = append(dependents[dep], id)
				}
			}
		}
	}

	var phases [][]Event

	for len(inDegree) > 0 {
		// Collect all resources with in-degree 0
		var ready []Identifier
		for id, deg := range inDegree {
			if deg == 0 {
				ready = append(ready, id)
			}
		}
		if len(ready) == 0 {
			// Cycle detected — emit remaining as a single phase
			var remaining []Event
			for id := range inDegree {
				remaining = append(remaining, eventSet[id])
			}
			phases = append(phases, remaining)
			break
		}

		phase := make([]Event, 0, len(ready))
		for _, id := range ready {
			phase = append(phase, eventSet[id])
			delete(inDegree, id)
			for _, dependent := range dependents[id] {
				inDegree[dependent]--
			}
		}
		phases = append(phases, phase)
	}

	return phases
}
