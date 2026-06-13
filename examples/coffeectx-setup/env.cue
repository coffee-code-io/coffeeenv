// Full declarative coffeectx setup for Claude on the global-state framework.
// Embed the agent target (claude.#Claude) and coffeectx.#Setup, then write data
// into the shared namespaces. coffeectx.#Setup reads `coffeectx.*` (config +
// jobs) and `agent.*`, generates ~/.coffeecode/config.yaml, installs skills/jobs,
// and feeds agent.mcps so Claude registers the MCP server.
//
// Projects are entered interactively via @inputMap: `apply` asks for a project
// name (the map key), then that project's repoPath / language / skills / jobs,
// then whether to add another. Non-interactively, supply entries by path, e.g.
//   --value 'coffeectx.projects.myrepo.repoPath=/r' ...
package env

import (
	"coffeeenv.dev/lib/agent/claude"
	// Aliased: the chart writes a `coffeectx:` data field, which would otherwise
	// collide with the imported package name.
	cctx "coffeeenv.dev/lib/coffeectx"
)

claude.#Claude
cctx.#Setup

// A coffeectx job, registered by writing data (installed into ~/.coffeecode/jobs).
// Registered jobs are the option set for each project's `jobs` @multichoice.
coffeectx: jobs: reindex: {
	description: "Re-index the repo"
	body:        "---\nname: reindex\ndescription: Re-index the repo\n---\n\nRebuild the knowledge graph."
}

// Registered skills are the option set for each project's `skills` @multichoice.
agent: skills: {
	api: {body: "---\nname: api\ndescription: API conventions\n---\n\nFollow the API guidelines."}
	contract: {body: "---\nname: contract\ndescription: Contract tests\n---\n\nKeep contracts green."}
}
