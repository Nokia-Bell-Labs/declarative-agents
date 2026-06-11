# Tool Definition Prompt

You are designing a tool for an agent. A tool is a word in a language the agent speaks. The agent is a state machine. Its behavior is a sequence of words — each word is a CLI command that does one thing deterministically.

## How to think about tools

A tool is not a script. A tool is not a workflow. A tool is a single verb with a clear meaning.

Think of it like a human language. "Cut" is a word. "Cut the bread into slices and put them on the plate" is a sentence. You are defining words, not sentences. Sentences are composed later by the agent from the words you define.

A good tool:

- Does one thing. If you need "and" to describe it, it's two tools.
- Is deterministic. Same inputs, same outputs. No LLM inside.
- Has a clear input and a clear output. Both are structured and typed.
- Declares what it changes in the world. Files written, state mutated, APIs called.
- Knows whether it can undo what it did. If it can, it says how. If it can't, it says so.
- Fails loudly and specifically. Never silently succeeds with wrong results.

A bad tool:

- Does multiple things depending on flags. That's multiple tools hiding in a trenchcoat.
- Requires an LLM to interpret its output. If the output needs judgment, the tool isn't finished — it should have made the judgment itself.
- Has side effects it doesn't declare. If it writes a file but doesn't say so, rollback can't find it.
- Is named for how it's implemented rather than what it does. "run-python-script" is a bad name. "parse-csv" is a good name.

## How to define a tool

A tool definition is a structured requirements document. It has:

**Problem**: Why does this tool need to exist? What gap in the agent's vocabulary does it fill? One paragraph. If you can't articulate the gap, the tool shouldn't exist.

**Goals**: What does success look like? Numbered. Measurable.

**Requirements**: What must the tool do? Grouped and numbered. Every requirement starts with "must" and describes one observable behavior. Requirements cover:

- Input: what it accepts, what formats, what defaults
- Output: what it produces, what structure, where it goes
- Side effects: what it changes in the world beyond its output
- Undo: how to reverse those side effects, or a statement that it can't be reversed
- Errors: how it fails and what it reports

**Non-goals**: What this tool deliberately does not do. This is how the agent knows not to misuse it. Be specific. "Does not transform data" is better than "is not a general-purpose tool."

**Acceptance criteria**: Concrete scenarios with inputs and expected outputs. These serve two purposes: they become tests, and they become examples the agent reads when deciding how to use the tool. Write them as if you're showing someone how the tool works.

**Reversibility**: One of three classifications:

- Reversible: the tool can undo its own effects given a record of what it did.
- Compensatable: the tool cannot literally undo but can issue a corrective action.
- Irreversible: the tool's effects cannot be undone. The agent must confirm before using it.

## The test for a good definition

Read your requirements and ask:

1. Could a developer implement this without asking any questions? If no, the requirements are incomplete.
2. Could the agent decide when to use this tool just by reading the problem, goals, and non-goals? If no, the problem statement is unclear.
3. Could the agent compose this tool with others in a sequence without ambiguity about what each step produces? If no, the output specification is incomplete.
4. If this tool fails mid-execution, does the definition say what state the world is in? If no, the error and undo requirements are incomplete.

## Relationship to other tools

No tool exists alone. When defining a tool, state:

- What tools typically come before it in a sequence (its likely inputs come from where?)
- What tools typically come after it (its output feeds into what?)
- What tools it overlaps with and how they differ

This is not prescriptive — the agent composes tools freely. But it helps the agent reason about when this tool is the right word to use versus a similar one.
