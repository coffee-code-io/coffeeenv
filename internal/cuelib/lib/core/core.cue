// Package core defines the executable contract shared by every chart target.
// A `#Main` is anything that contributes to the global `states` map; composing
// charts is just unioning their #Main values (each agent/feature target is a
// #Main, and a chart's own #Main embeds the ones it wants). It also carries the
// venv PATH state, so a composition installs into <root>/node_modules/.bin under
// the local engine regardless of which targets are present.
package core

import (
	"coffeeenv.dev/lib/context"
	st "coffeeenv.dev/lib/states"
)

// #Main is the executable contract: an open struct producing the `states` map.
#Main: {
	states: {[string]: st.#State}
	if context.engine == "local" {
		states: "PATH": st.#EnvState & {
			value:  "\(context.root)/node_modules/.bin:$PATH"
			expand: true
			target: "\(context.root)/env.sh"
		}
	}
	...
}
