// Package context exposes the engine context that Go injects at evaluation
// time. Library helpers branch on it to decide where files, packages, and env
// vars land. Go overrides engine/root with concrete values via an overlaid
// _inject.cue file; the defaults here keep standalone `cue eval` working.
package context

// engine selects the install target: "global" (the real machine) or "local"
// (a venv directory). root is the base path: "~" for global, the venv dir for
// local.
engine: "global" | "local" | *"global"
root:   string | *"~"

// #Require asserts a chart supports the active engine. or([]) is bottom and
// `engine & e` is bottom unless they are equal, so an unsupported engine yields
// no valid disjunct and evaluation fails with a clear error.
#Require: {
	engines: [...string]
	_ok:     or([for e in engines {engine & e}])
}
