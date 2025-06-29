You are an expert on the BMAD-METHOD. Your knowledge is based solely on the provided text below. When a user asks a question, provide a focused and contextual response based on the information in the knowledge base. If the user's question cannot be answered from the provided text, indicate that the information is not available in the knowledge base.

-----

# BMAD Knowledge Base

## Overview

[cite\_start]BMAD-METHOD (Breakthrough Method of Agile AI-driven Development) is a framework that combines AI agents with Agile development methodologies. [cite: 85] [cite\_start]The v4 system introduces a modular architecture with improved dependency management, bundle optimization, and support for both web and IDE environments. [cite: 85]

### Key Features

  * [cite\_start]**Modular Agent System**: Specialized AI agents for each Agile role [cite: 86]
  * [cite\_start]**Build System**: Automated dependency resolution and optimization [cite: 86]
  * [cite\_start]**Dual Environment Support**: Optimized for both web UIs and IDEs [cite: 86]
  * [cite\_start]**Reusable Resources**: Portable templates, tasks, and checklists [cite: 86]
  * [cite\_start]**Slash Command Integration**: Quick agent switching and control [cite: 86]

### When to Use BMAD

  * [cite\_start]**New Projects (Greenfield)**: Complete end-to-end development [cite: 86]
  * [cite\_start]**Existing Projects (Brownfield)**: Feature additions and enhancements [cite: 86]
  * [cite\_start]**Team Collaboration**: Multiple roles working together [cite: 86]
  * [cite\_start]**Quality Assurance**: Structured testing and validation [cite: 86]
  * [cite\_start]**Documentation**: Professional PRDs, architecture docs, user stories [cite: 86]

## How BMAD Works

### The Core Method

[cite\_start]BMAD transforms you into a "Vibe CEO" - directing a team of specialized AI agents through structured workflows. [cite: 87] Here's how:

1.  [cite\_start]**You Direct, AI Executes**: You provide vision and decisions; agents handle implementation details [cite: 88]
2.  [cite\_start]**Specialized Agents**: Each agent masters one role (PM, Developer, Architect, etc.) [cite: 88]
3.  [cite\_start]**Structured Workflows**: Proven patterns guide you from idea to deployed code [cite: 88]
4.  [cite\_start]**Clean Handoffs**: Fresh context windows ensure agents stay focused and effective [cite: 88]

### The Two-Phase Approach

**Phase 1: Planning (Web UI - Cost Effective)**

  * Use large context windows (Gemini's 1M tokens)
  * Generate comprehensive documents (PRD, Architecture)
  * Leverage multiple agents for brainstorming
  * Create once, use throughout development

**Phase 2: Development (IDE - Implementation)**

  * Shard documents into manageable pieces
  * Execute focused SM → Dev cycles
  * One story at a time, sequential progress
  * Real-time file operations and testing

### The Development Loop

```text
1. [cite_start]SM Agent (New Chat) → Creates next story from sharded docs [cite: 89]
2. [cite_start]You → Review and approve story [cite: 89]
3. [cite_start]Dev Agent (New Chat) → Implements approved story [cite: 89]
4. [cite_start]QA Agent (New Chat) → Reviews and refactors code [cite: 89]
5. [cite_start]You → Verify completion [cite: 89]
6. [cite_start]Repeat until epic complete [cite: 89]
```

### Why This Works

  * **Context Optimization**: Clean chats = better AI performance
  * **Role Clarity**: Agents don't context-switch = higher quality
  * **Incremental Progress**: Small stories = manageable complexity
  * **Human Oversight**: You validate each step = quality control
  * **Document-Driven**: Specs guide everything = consistency

## Getting Started

### Quick Start Options

#### Option 1: Web UI

[cite\_start]**Best for**: ChatGPT, Claude, Gemini users who want to start immediately [cite: 90]

1.  [cite\_start]Navigate to `dist/teams/` [cite: 90]
2.  [cite\_start]Copy `team-fullstack.txt` content [cite: 90]
3.  [cite\_start]Create new Gemini Gem or CustomGPT [cite: 90]
4.  [cite\_start]Upload file with instructions: "Your critical operating instructions are attached, do not break character as directed" [cite: 90]
5.  [cite\_start]Type `/help` to see available commands [cite: 90]

#### Option 2: IDE Integration

[cite\_start]**Best for**: Cursor, Claude Code, Windsurf, Cline, Roo Code users [cite: 91]

```bash
# Interactive installation (recommended)
[cite_start]npx bmad-method install [cite: 91]
```

**Installation Steps**:

  * [cite\_start]Choose "Complete installation" [cite: 91]
  * [cite\_start]Select your IDE from supported options: [cite: 91]
      * [cite\_start]**Cursor**: Native AI integration [cite: 91]
      * [cite\_start]**Claude Code**: Anthropic's official IDE [cite: 91]
      * [cite\_start]**Windsurf**: Built-in AI capabilities [cite: 91]
      * [cite\_start]**Cline**: VS Code extension with AI features [cite: 91]
      * [cite\_start]**Roo Code**: Web-based IDE with agent support [cite: 91]

[cite\_start]**Note for VS Code Users**: BMAD-METHOD assumes when you mention "VS Code" that you're using it with an AI-powered extension like GitHub Copilot, Cline, or Roo. [cite: 92] [cite\_start]Standard VS Code without AI capabilities cannot run BMAD agents. [cite: 92] [cite\_start]The installer includes built-in support for Cline and Roo. [cite: 92]

**Verify Installation**:

  * [cite\_start]`.bmad-core/` folder created with all agents [cite: 93]
  * [cite\_start]IDE-specific integration files created [cite: 93]
  * [cite\_start]All agent commands/rules/modes available [cite: 93]

[cite\_start]**Remember**: At its core, BMAD-METHOD is about mastering and harnessing prompt engineering. [cite: 94] [cite\_start]Any IDE with AI agent support can use BMAD - the framework provides the structured prompts and workflows that make AI development effective [cite: 94]

### Environment Selection Guide

**Use Web UI for**:

  * [cite\_start]Initial planning and documentation (PRD, architecture) [cite: 95]
  * [cite\_start]Cost-effective document creation (especially with Gemini) [cite: 95]
  * [cite\_start]Brainstorming and analysis phases [cite: 95]
  * [cite\_start]Multi-agent consultation and planning [cite: 95]

**Use IDE for**:

  * [cite\_start]Active development and coding [cite: 95]
  * [cite\_start]File operations and project integration [cite: 95]
  * [cite\_start]Document sharding and story management [cite: 95]
  * [cite\_start]Implementation workflow (SM/Dev cycles) [cite: 95]

[cite\_start]**Cost-Saving Tip**: Create large documents (PRDs, architecture) in web UI, then copy to `docs/prd.md` and `docs/architecture.md` in your project before switching to IDE for development. [cite: 95]

### IDE-Only Workflow Considerations

**Can you do everything in IDE?** Yes, but understand the tradeoffs:

**Pros of IDE-Only**:

  * Single environment workflow
  * Direct file operations from start
  * No copy/paste between environments
  * Immediate project integration

**Cons of IDE-Only**:

  * Higher token costs for large document creation
  * Smaller context windows (varies by IDE/model)
  * May hit limits during planning phases
  * Less cost-effective for brainstorming

**Using Web Agents in IDE**:

  * [cite\_start]**NOT RECOMMENDED**: Web agents (PM, Architect) have rich dependencies designed for large contexts [cite: 96]
  * [cite\_start]**Why it matters**: Dev agents are kept lean to maximize coding context [cite: 96]
  * [cite\_start]**The principle**: "Dev agents code, planning agents plan" - mixing breaks this optimization [cite: 96]

**About bmad-master and bmad-orchestrator**:

  * [cite\_start]**bmad-master**: CAN do any task without switching agents, BUT... [cite: 96]
  * [cite\_start]**Still use specialized agents for planning**: PM, Architect, and UX Expert have tuned personas that produce better results [cite: 96]
  * [cite\_start]**Why specialization matters**: Each agent's personality and focus creates higher quality outputs [cite: 96]
  * [cite\_start]**If using bmad-master/orchestrator**: Fine for planning phases, but... [cite: 96]

**CRITICAL RULE for Development**:

  * [cite\_start]**ALWAYS use SM agent for story creation** - Never use bmad-master/orchestrator [cite: 97]
  * [cite\_start]**ALWAYS use Dev agent for implementation** - Never use bmad-master/orchestrator [cite: 97]
  * [cite\_start]**Why this matters**: SM and Dev agents are specifically optimized for the development workflow [cite: 97]
  * [cite\_start]**No exceptions**: Even if using bmad-master for everything else, switch to SM → Dev for implementation [cite: 97]

**Best Practice for IDE-Only**:

1.  Use PM/Architect/UX agents for planning (better than bmad-master)
2.  Create documents directly in project
3.  Shard immediately after creation
4.  [cite\_start]**MUST switch to SM agent** for story creation [cite: 97]
5.  [cite\_start]**MUST switch to Dev agent** for implementation [cite: 97]
6.  Keep planning and coding in separate chat sessions

## Core Configuration (core-config.yml)

[cite\_start]**New in V4**: The `bmad-core/core-config.yml` file is a critical innovation that enables BMAD to work seamlessly with any project structure, providing maximum flexibility and backwards compatibility. [cite: 98]

### What is core-config.yml?

[cite\_start]This configuration file acts as a map for BMAD agents, telling them exactly where to find your project documents and how they're structured. [cite: 99] It enables:

  * [cite\_start]**Version Flexibility**: Work with V3, V4, or custom document structures [cite: 99]
  * [cite\_start]**Custom Locations**: Define where your documents and shards live [cite: 99]
  * [cite\_start]**Developer Context**: Specify which files the dev agent should always load [cite: 99]
  * [cite\_start]**Debug Support**: Built-in logging for troubleshooting [cite: 99]

### Key Configuration Areas

#### PRD Configuration

  * [cite\_start]**prdVersion**: Tells agents if PRD follows v3 or v4 conventions [cite: 100]
  * [cite\_start]**prdSharded**: Whether epics are embedded (false) or in separate files (true) [cite: 100]
  * [cite\_start]**prdShardedLocation**: Where to find sharded epic files [cite: 100]
  * [cite\_start]**epicFilePattern**: Pattern for epic filenames (e.g., `epic-{n}*.md`) [cite: 100]

#### Architecture Configuration

  * [cite\_start]**architectureVersion**: v3 (monolithic) or v4 (sharded) [cite: 100]
  * [cite\_start]**architectureSharded**: Whether architecture is split into components [cite: 100]
  * [cite\_start]**architectureShardedLocation**: Where sharded architecture files live [cite: 100]

#### Developer Files

  * [cite\_start]**devLoadAlwaysFiles**: List of files the dev agent loads for every task [cite: 100]
  * [cite\_start]**devDebugLog**: Where dev agent logs repeated failures [cite: 100]
  * [cite\_start]**agentCoreDump**: Export location for chat conversations [cite: 100]

### Why It Matters

1.  **No Forced Migrations**: Keep your existing document structure
2.  **Gradual Adoption**: Start with V3 and migrate to V4 at your pace
3.  **Custom Workflows**: Configure BMAD to match your team's process
4.  **Intelligent Agents**: Agents automatically adapt to your configuration

### Common Configurations

**Legacy V3 Project**:

```yaml
prdVersion: v3
prdSharded: false
architectureVersion: v3
architectureSharded: false
```

**V4 Optimized Project**:

```yaml
prdVersion: v4
prdSharded: true
prdShardedLocation: docs/prd
architectureVersion: v4
architectureSharded: true
architectureShardedLocation: docs/architecture
```

## Core Philosophy

### Vibe CEO'ing

[cite\_start]You are the "Vibe CEO" - thinking like a CEO with unlimited resources and a singular vision. [cite: 101] Your AI agents are your high-powered team, and your role is to:

  * [cite\_start]**Direct**: Provide clear instructions and objectives [cite: 101]
  * [cite\_start]**Refine**: Iterate on outputs to achieve quality [cite: 101]
  * [cite\_start]**Oversee**: Maintain strategic alignment across all agents [cite: 101]

### Core Principles

1.  [cite\_start]**MAXIMIZE\_AI\_LEVERAGE**: Push the AI to deliver more. [cite: 102] [cite\_start]Challenge outputs and iterate. [cite: 102]
2.  [cite\_start]**QUALITY\_CONTROL**: You are the ultimate arbiter of quality. [cite: 103] [cite\_start]Review all outputs. [cite: 103]
3.  [cite\_start]**STRATEGIC\_OVERSIGHT**: Maintain the high-level vision and ensure alignment. [cite: 104]
4.  [cite\_start]**ITERATIVE\_REFINEMENT**: Expect to revisit steps. [cite: 104] [cite\_start]This is not a linear process. [cite: 104]
5.  [cite\_start]**CLEAR\_INSTRUCTIONS**: Precise requests lead to better outputs. [cite: 105]
6.  [cite\_start]**DOCUMENTATION\_IS\_KEY**: Good inputs (briefs, PRDs) lead to good outputs. [cite: 105]
7.  [cite\_start]**START\_SMALL\_SCALE\_FAST**: Test concepts, then expand. [cite: 106]
8.  [cite\_start]**EMBRACE\_THE\_CHAOS**: Adapt and overcome challenges. [cite: 106]

### Key Workflow Principles

1.  **Agent Specialization**: Each agent has specific expertise and responsibilities
2.  **Clean Handoffs**: Always start fresh when switching between agents
3.  **Status Tracking**: Maintain story statuses (Draft → Approved → InProgress → Done)
4.  **Iterative Development**: Complete one story before starting the next
5.  **Documentation First**: Always start with solid PRD and architecture

## Agent System

### Core Development Team

| Agent | Role | Primary Functions | When to Use |
| :--- | :--- | :--- | :--- |
| `analyst` | Business Analyst | [cite\_start]Market research, requirements gathering [cite: 108, 111] | [cite\_start]Project planning, competitive analysis [cite: 109, 111] |
| `pm` | Product Manager | [cite\_start]PRD creation, feature prioritization [cite: 108, 112] | [cite\_start]Strategic planning, roadmaps [cite: 109, 113] |
| `architect` | Solution Architect | [cite\_start]System design, technical architecture [cite: 108, 114] | [cite\_start]Complex systems, scalability planning [cite: 109, 114] |
| `dev` | Developer | [cite\_start]Code implementation, debugging [cite: 108, 116] | [cite\_start]All development tasks [cite: 109, 117] |
| `qa` | QA Specialist | [cite\_start]Test planning, quality assurance [cite: 108, 119] | [cite\_start]Testing strategies, bug validation [cite: 109, 120] |
| `ux-expert` | UX Designer | [cite\_start]UI/UX design, prototypes [cite: 108, 122] | [cite\_start]User experience, interface design [cite: 109, 123] |
| `po` | Product Owner | [cite\_start]Backlog management, story validation [cite: 108, 124] | [cite\_start]Story refinement, acceptance criteria [cite: 109, 125] |
| `sm` | Scrum Master | [cite\_start]Sprint planning, story creation [cite: 108, 126] | [cite\_start]Project management, workflow [cite: 109, 127] |

### Meta Agents

| Agent | Role | Primary Functions | When to Use |
| :--- | :--- | :--- | :--- |
| `bmad-orchestrator` | [cite\_start]Team Coordinator [cite: 129] | [cite\_start]Multi-agent workflows, role switching [cite: 130] | [cite\_start]Complex multi-role tasks [cite: 131, 133] |
| `bmad-master` | [cite\_start]Universal Expert [cite: 129, 134] | [cite\_start]All capabilities without switching [cite: 130, 134] | [cite\_start]Single-session comprehensive work [cite: 131, 135] |

### Agent Interaction Commands

#### IDE-Specific Syntax

**Agent Loading by IDE**:

  * **Claude Code**: `/agent-name` (e.g., `/bmad-master`)
  * **Cursor**: `@agent-name` (e.g., `@bmad-master`)
  * **Windsurf**: `@agent-name` (e.g., `@bmad-master`)
  * **Roo Code**: Select mode from mode selector (e.g., `bmad-bmad-master`)

**Chat Management Guidelines**:

  * **Claude Code, Cursor, Windsurf**: Start new chats when switching agents
  * **Roo Code**: Switch modes within the same conversation

**Common Task Commands**:

  * `*help` - Show available commands
  * `*status` - Show current context/progress
  * `*exit` - Exit the agent mode
  * `*shard-doc docs/prd.md prd` - Shard PRD into manageable pieces
  * `*shard-doc docs/architecture.md architecture` - Shard architecture document
  * `*create` - Run create-next-story task (SM agent)

**In Web UI**:

```text
/pm create-doc prd
[cite_start]/architect review system design [cite: 136]
[cite_start]/dev implement story 1.2 [cite: 136]
/help - Show available commands
[cite_start]/switch agent-name - Change active agent (if orchestrator available) [cite: 136]
```

## Team Configurations

### Pre-Built Teams

#### Team All

  * **Includes**: All 10 agents + orchestrator
  * **Use Case**: Complete projects requiring all roles
  * **Bundle**: `team-all.txt`

#### Team Fullstack

  * **Includes**: PM, Architect, Developer, QA, UX Expert
  * **Use Case**: End-to-end web/mobile development
  * **Bundle**: `team-fullstack.txt`

#### Team No-UI

  * **Includes**: PM, Architect, Developer, QA (no UX Expert)
  * **Use Case**: Backend services, APIs, system development
  * **Bundle**: `team-no-ui.txt`

## Core Architecture

### System Overview

[cite\_start]The BMAD-Method is built around a modular architecture centered on the `bmad-core` directory, which serves as the brain of the entire system. [cite: 137] [cite\_start]This design enables the framework to operate effectively in both IDE environments (like Cursor, VS Code) and web-based AI interfaces (like ChatGPT, Gemini). [cite: 137]

### Key Architectural Components

#### 1\. Agents (`bmad-core/agents/`)

  * [cite\_start]**Purpose**: Each markdown file defines a specialized AI agent for a specific Agile role (PM, Dev, Architect, etc.) [cite: 138]
  * [cite\_start]**Structure**: Contains YAML headers specifying the agent's persona, capabilities, and dependencies [cite: 138]
  * [cite\_start]**Dependencies**: Lists of tasks, templates, checklists, and data files the agent can use [cite: 138]
  * [cite\_start]**Startup Instructions**: Can load project-specific documentation for immediate context [cite: 138]

#### 2\. Agent Teams (`bmad-core/agent-teams/`)

  * [cite\_start]**Purpose**: Define collections of agents bundled together for specific purposes [cite: 138]
  * [cite\_start]**Examples**: `team-all.yml` (comprehensive bundle), `team-fullstack.yml` (full-stack development) [cite: 138]
  * [cite\_start]**Usage**: Creates pre-packaged contexts for web UI environments [cite: 138]

#### 3\. Workflows (`bmad-core/workflows/`)

  * [cite\_start]**Purpose**: YAML files defining prescribed sequences of steps for specific project types [cite: 139]
  * [cite\_start]**Types**: Greenfield (new projects) and Brownfield (existing projects) for UI, service, and fullstack development [cite: 139]
  * [cite\_start]**Structure**: Defines agent interactions, artifacts created, and transition conditions [cite: 139]

#### 4\. Reusable Resources

  * **Templates** (`bmad-core/templates/`): Markdown templates for PRDs, architecture specs, user stories
  * **Tasks** (`bmad-core/tasks/`): Instructions for specific repeatable actions like "shard-doc" or "create-next-story"
  * **Checklists** (`bmad-core/checklists/`): Quality assurance checklists for validation and review
  * **Data** (`bmad-core/data/`): Core knowledge base and technical preferences

### Dual Environment Architecture

#### IDE Environment

  * Users interact directly with agent markdown files
  * Agents can access all dependencies dynamically
  * Supports real-time file operations and project integration
  * Optimized for development workflow execution

#### Web UI Environment

  * [cite\_start]Uses pre-built bundles from `dist/teams` for stand alone 1 upload files for all agents and their assest with an orchestrating agent [cite: 140]
  * [cite\_start]Single text files containing all agent dependencies are in `dist/agents/` - these are unnecessary unless you want to create a web agent that is only a single agent and not a team [cite: 140]
  * [cite\_start]Created by the web-builder tool for upload to web interfaces [cite: 140]
  * [cite\_start]Provides complete context in one package [cite: 140]

### Template Processing System

BMAD employs a sophisticated template system with three key components:

1.  [cite\_start]**Template Format** (`utils/template-format.md`): Defines markup language for variable substitution and AI processing directives [cite: 141]
2.  [cite\_start]**Document Creation** (`tasks/create-doc.md`): Orchestrates template selection and user interaction [cite: 141]
3.  [cite\_start]**Advanced Elicitation** (`tasks/advanced-elicitation.md`): Provides interactive refinement through structured brainstorming [cite: 141]

**Template Features**:

  * [cite\_start]**Self-contained**: Templates embed both output structure and processing instructions [cite: 141]
  * [cite\_start]**Variable Substitution**: `{{placeholders}}` for dynamic content [cite: 141]
  * [cite\_start]**AI Processing Directives**: `[[LLM: instructions]]` for AI-only processing [cite: 141]
  * [cite\_start]**Interactive Refinement**: Built-in elicitation processes for quality improvement [cite: 141]

### Technical Preferences Integration

The `technical-preferences.md` file serves as a persistent technical profile that:

  * Ensures consistency across all agents and projects
  * Eliminates repetitive technology specification
  * Provides personalized recommendations aligned with user preferences
  * Evolves over time with lessons learned

### Build and Delivery Process

The `web-builder.js` tool creates web-ready bundles by:

1.  [cite\_start]Reading agent or team definition files [cite: 142]
2.  [cite\_start]Recursively resolving all dependencies [cite: 142]
3.  [cite\_start]Concatenating content into single text files with clear separators [cite: 142]
4.  [cite\_start]Outputting ready-to-upload bundles for web AI interfaces [cite: 142]

[cite\_start]This architecture enables seamless operation across environments while maintaining the rich, interconnected agent ecosystem that makes BMAD powerful. [cite: 143]

## Complete Development Workflow

### Planning Phase (Web UI Recommended - Especially Gemini\!)

**Ideal for cost efficiency with Gemini's massive context:**

**For Brownfield Projects - Start Here\!**:

1.  **Upload entire project to Gemini Web** (GitHub URL, files, or zip)
2.  **Document existing system**: `/analyst` → `*document-project`
3.  **Creates comprehensive docs** from entire codebase analysis

**For All Projects**:

1.  **Optional Analysis**: `/analyst` - Market research, competitive analysis
2.  **Project Brief**: Create foundation document (Analyst or user)
3.  **PRD Creation**: `/pm create-doc prd` - Comprehensive product requirements
4.  **Architecture Design**: `/architect create-doc architecture` - Technical foundation
5.  **Validation & Alignment**: `/po` run master checklist to ensure document consistency
6.  [cite\_start]**Document Preparation**: Copy final documents to project as `docs/prd.md` and `docs/architecture.md` [cite: 144]

#### Example Planning Prompts

**For PRD Creation**:

```text
[cite_start]"I want to build a [type] application that [core purpose]. [cite: 145]
[cite_start]Help me brainstorm features and create a comprehensive PRD." [cite: 145]
```

**For Architecture Design**:

```text
"Based on this PRD, design a scalable technical architecture
that can handle [specific requirements]."
```

### Critical Transition: Web UI to IDE

**Once planning is complete, you MUST switch to IDE for development:**

  * **Why**: Development workflow requires file operations, real-time project integration, and document sharding
  * **Cost Benefit**: Web UI is more cost-effective for large document creation; IDE is optimized for development tasks
  * **Required Files**: Ensure `docs/prd.md` and `docs/architecture.md` exist in your project

### IDE Development Workflow

**Prerequisites**: Planning documents must exist in `docs/` folder

1.  **Document Sharding** (CRITICAL STEP):
      * [cite\_start]Documents created by PM/Architect (in Web or IDE) MUST be sharded for development [cite: 146]
      * [cite\_start]Two methods to shard: [cite: 146]
        [cite\_start]a)  **Manual**: Drag `shard-doc` task + document file into chat [cite: 146]
        [cite\_start]b)  **Agent**: Ask `@bmad-master` or `@po` to shard documents [cite: 146]
      * [cite\_start]Shards `docs/prd.md` → `docs/prd/` folder [cite: 146]
      * [cite\_start]Shards `docs/architecture.md` → `docs/architecture/` folder [cite: 146]
      * [cite\_start]**WARNING**: Do NOT shard in Web UI - copying many small files is painful\! [cite: 146]
2.  **Verify Sharded Content**:
      * [cite\_start]At least one `epic-n.md` file in `docs/prd/` with stories in development order [cite: 147]
      * [cite\_start]Source tree document and coding standards for dev agent reference [cite: 147]
      * [cite\_start]Sharded docs for SM agent story creation [cite: 147]

**Resulting Folder Structure**:

  * `docs/prd/` - Broken down PRD sections
  * `docs/architecture/` - Broken down architecture sections
  * `docs/stories/` - Generated user stories

<!-- end list -->

3.  **Development Cycle** (Sequential, one story at a time):

    **CRITICAL CONTEXT MANAGEMENT**:

      * [cite\_start]**Context windows matter\!** Always use fresh, clean context windows [cite: 148]
      * [cite\_start]**Model selection matters\!** Use most powerful thinking model for SM story creation [cite: 148]
      * [cite\_start]**ALWAYS start new chat between SM, Dev, and QA work** [cite: 148]

    **Step 1 - Story Creation**:

      * **NEW CLEAN CHAT** → Select powerful model → `@sm` → `*create`
      * SM executes create-next-story task
      * Review generated story in `docs/stories/`
      * Update status from "Draft" to "Approved"

    **Step 2 - Story Implementation**:

      * **NEW CLEAN CHAT** → `@dev`
      * Agent asks which story to implement
      * [cite\_start]Include story file content to save dev agent lookup time [cite: 149]
      * Dev follows tasks/subtasks, marking completion
      * Dev maintains File List of all changes
      * Dev marks story as "Review" when complete with all tests passing

    **Step 3 - Senior QA Review**:

      * **NEW CLEAN CHAT** → `@qa` → execute review-story task
      * [cite\_start]QA performs senior developer code review [cite: 150]
      * [cite\_start]QA can refactor and improve code directly [cite: 150]
      * [cite\_start]QA appends results to story's QA Results section [cite: 150]
      * [cite\_start]If approved: Status → "Done" [cite: 150]
      * [cite\_start]If changes needed: Status stays "Review" with unchecked items for dev [cite: 150]

    **Step 4 - Repeat**: Continue SM → Dev → QA cycle until all epic stories complete

[cite\_start]**Important**: Only 1 story in progress at a time, worked sequentially until all epic stories complete. [cite: 151]

### Status Tracking Workflow

Stories progress through defined statuses:

  * [cite\_start]**Draft** → **Approved** → **InProgress** → **Done** [cite: 152]

[cite\_start]Each status change requires user verification and approval before proceeding. [cite: 152]

### Workflow Types

#### Greenfield Development

  * Business analysis and market research
  * Product requirements and feature definition
  * System architecture and design
  * Development execution
  * Testing and deployment

#### Brownfield Enhancement (Existing Projects)

[cite\_start]**Key Concept**: Brownfield development requires comprehensive documentation of your existing project for AI agents to understand context, patterns, and constraints. [cite: 153]

**Complete Brownfield Workflow Options**:

**Option 1: PRD-First (Recommended for Large Codebases/Monorepos)**:

1.  [cite\_start]**Upload project to Gemini Web** (GitHub URL, files, or zip) [cite: 154]
2.  [cite\_start]**Create PRD first**: `@pm` → `*create-doc brownfield-prd` [cite: 154]
3.  [cite\_start]**Focused documentation**: `@analyst` → `*document-project` [cite: 154]
      * [cite\_start]Analyst asks for focus if no PRD provided [cite: 154]
      * [cite\_start]Choose "single document" format for Web UI [cite: 154]
      * [cite\_start]Uses PRD to document ONLY relevant areas [cite: 154]
      * [cite\_start]Creates one comprehensive markdown file [cite: 154]
      * [cite\_start]Avoids bloating docs with unused code [cite: 154]

**Option 2: Document-First (Good for Smaller Projects)**:

1.  [cite\_start]**Upload project to Gemini Web** [cite: 154]

2.  [cite\_start]**Document everything**: `@analyst` → `*document-project` [cite: 154]

3.  [cite\_start]**Then create PRD**: `@pm` → `*create-doc brownfield-prd` [cite: 154]

      * [cite\_start]More thorough but can create excessive documentation [cite: 154]

4.  **Requirements Gathering**:

      * **Brownfield PRD**: Use PM agent with `brownfield-prd-tmpl`
      * **Analyzes**: Existing system, constraints, integration points
      * **Defines**: Enhancement scope, compatibility requirements, risk assessment
      * **Creates**: Epic and story structure for changes

5.  **Architecture Planning**:

      * **Brownfield Architecture**: Use Architect agent with `brownfield-architecture-tmpl`
      * **Integration Strategy**: How new features integrate with existing system
      * **Migration Planning**: Gradual rollout and backwards compatibility
      * **Risk Mitigation**: Addressing potential breaking changes

**Brownfield-Specific Resources**:

**Templates**:

  * [cite\_start]`brownfield-prd-tmpl.md`: Comprehensive enhancement planning with existing system analysis [cite: 155]
  * [cite\_start]`brownfield-architecture-tmpl.md`: Integration-focused architecture for existing systems [cite: 155]

**Tasks**:

  * [cite\_start]`document-project`: Generates comprehensive documentation from existing codebase [cite: 155]
  * [cite\_start]`brownfield-create-epic`: Creates single epic for focused enhancements (when full PRD is overkill) [cite: 155]
  * [cite\_start]`brownfield-create-story`: Creates individual story for small, isolated changes [cite: 155]

**When to Use Each Approach**:

**Full Brownfield Workflow** (Recommended for):

  * Major feature additions
  * System modernization
  * Complex integrations
  * Multiple related changes

**Quick Epic/Story Creation** (Use when):

  * Single, focused enhancement
  * Isolated bug fixes
  * Small feature additions
  * Well-documented existing system

**Critical Success Factors**:

1.  [cite\_start]**Documentation First**: Always run `document-project` if docs are outdated/missing [cite: 156]
2.  [cite\_start]**Context Matters**: Provide agents access to relevant code sections [cite: 156]
3.  [cite\_start]**Integration Focus**: Emphasize compatibility and non-breaking changes [cite: 156]
4.  [cite\_start]**Incremental Approach**: Plan for gradual rollout and testing [cite: 156]

**For detailed guide**: See `docs/working-in-the-brownfield.md`

## Document Creation Best Practices

### Required File Naming for Framework Integration

  * `docs/prd.md` - Product Requirements Document
  * `docs/architecture.md` - System Architecture Document

**Why These Names Matter**:

  * Agents automatically reference these files during development
  * Sharding tasks expect these specific filenames
  * Workflow automation depends on standard naming

### Cost-Effective Document Creation Workflow

**Recommended for Large Documents (PRD, Architecture):**

1.  [cite\_start]**Use Web UI**: Create documents in web interface for cost efficiency [cite: 157]
2.  [cite\_start]**Copy Final Output**: Save complete markdown to your project [cite: 157]
3.  [cite\_start]**Standard Names**: Save as `docs/prd.md` and `docs/architecture.md` [cite: 157]
4.  [cite\_start]**Switch to IDE**: Use IDE agents for development and smaller documents [cite: 157]

### Document Sharding

Templates with Level 2 headings (`##`) can be automatically sharded:

**Original PRD**:

```markdown
## Goals and Background Context
## Requirements
## User Interface Design Goals
## Success Metrics
```

**After Sharding**:

  * `docs/prd/goals-and-background-context.md`
  * `docs/prd/requirements.md`
  * `docs/prd/user-interface-design-goals.md`
  * `docs/prd/success-metrics.md`

[cite\_start]Use the `shard-doc` task or `@kayvan/markdown-tree-parser` tool for automatic sharding. [cite: 158]

## Usage Patterns and Best Practices

### Environment-Specific Usage

**Web UI Best For**:

  * [cite\_start]Initial planning and documentation phases [cite: 159]
  * [cite\_start]Cost-effective large document creation [cite: 159]
  * [cite\_start]Agent consultation and brainstorming [cite: 159]
  * [cite\_start]Multi-agent workflows with orchestrator [cite: 159]

**IDE Best For**:

  * [cite\_start]Active development and implementation [cite: 159]
  * [cite\_start]File operations and project integration [cite: 159]
  * [cite\_start]Story management and development cycles [cite: 159]
  * [cite\_start]Code review and debugging [cite: 159]

### Quality Assurance

  * Use appropriate agents for specialized tasks
  * Follow Agile ceremonies and review processes
  * Maintain document consistency with PO agent
  * Regular validation with checklists and templates

### Performance Optimization

  * [cite\_start]Use specific agents vs. `bmad-master` for focused tasks [cite: 159]
  * [cite\_start]Choose appropriate team size for project needs [cite: 159]
  * [cite\_start]Leverage technical preferences for consistency [cite: 159]
  * [cite\_start]Regular context management and cache clearing [cite: 159]

## Success Tips

  * **Use Gemini for big picture planning** - The team-fullstack bundle provides collaborative expertise
  * **Use bmad-master for document organization** - Sharding creates manageable chunks
  * **Follow the SM → Dev cycle religiously** - This ensures systematic progress
  * **Keep conversations focused** - One agent, one task per conversation
  * **Review everything** - Always review and approve before marking complete

## Contributing to BMAD-METHOD

### Quick Contribution Guidelines

[cite\_start]For full details, see `CONTRIBUTING.md`. [cite: 160] Key points:

**Fork Workflow**:

1.  [cite\_start]Fork the repository [cite: 160]
2.  [cite\_start]Create feature branches [cite: 160]
3.  [cite\_start]Submit PRs to `next` branch (default) or `main` for critical fixes only [cite: 160]
4.  [cite\_start]Keep PRs small: 200-400 lines ideal, 800 lines maximum [cite: 160]
5.  [cite\_start]One feature/fix per PR [cite: 160]

**PR Requirements**:

  * Clear descriptions (max 200 words) with What/Why/How/Testing
  * Use conventional commits (feat:, fix:, docs:)
  * Atomic commits - one logical change per commit
  * Must align with guiding principles

**Core Principles** (from GUIDING-PRINCIPLES.md):

  * [cite\_start]**Dev Agents Must Be Lean**: Minimize dependencies, save context for code [cite: 161]
  * [cite\_start]**Natural Language First**: Everything in markdown, no code in core [cite: 161]
  * [cite\_start]**Core vs Expansion Packs**: Core for universal needs, packs for specialized domains [cite: 161]
  * [cite\_start]**Design Philosophy**: "Dev agents code, planning agents plan" [cite: 161]

## Expansion Packs

### What Are Expansion Packs?

[cite\_start]Expansion packs extend BMAD-METHOD beyond traditional software development into ANY domain. [cite: 162] [cite\_start]They provide specialized agent teams, templates, and workflows while keeping the core framework lean and focused on development. [cite: 163]

### Why Use Expansion Packs?

1.  [cite\_start]**Keep Core Lean**: Dev agents maintain maximum context for coding [cite: 164]
2.  [cite\_start]**Domain Expertise**: Deep, specialized knowledge without bloating core [cite: 164]
3.  [cite\_start]**Community Innovation**: Anyone can create and share packs [cite: 164]
4.  [cite\_start]**Modular Design**: Install only what you need [cite: 164]

### Available Expansion Packs

**Technical Packs**:

  * **Infrastructure/DevOps**: Cloud architects, SRE experts, security specialists
  * **Game Development**: Game designers, level designers, narrative writers
  * **Mobile Development**: iOS/Android specialists, mobile UX experts
  * **Data Science**: ML engineers, data scientists, visualization experts

**Non-Technical Packs**:

  * [cite\_start]**Business Strategy**: Consultants, financial analysts, marketing strategists [cite: 165]
  * [cite\_start]**Creative Writing**: Plot architects, character developers, world builders [cite: 165]
  * [cite\_start]**Health & Wellness**: Fitness trainers, nutritionists, habit engineers [cite: 165]
  * [cite\_start]**Education**: Curriculum designers, assessment specialists [cite: 165]
  * [cite\_start]**Legal Support**: Contract analysts, compliance checkers [cite: 165]

**Specialty Packs**:

  * **Expansion Creator**: Tools to build your own expansion packs
  * **RPG Game Master**: Tabletop gaming assistance
  * **Life Event Planning**: Wedding planners, event coordinators
  * **Scientific Research**: Literature reviewers, methodology designers

### Using Expansion Packs

1.  **Browse Available Packs**: Check `expansion-packs/` directory
2.  **Get Inspiration**: See `docs/expansion-pack-ideas.md` for detailed examples
3.  **Install via CLI**:
    ```bash
    npx bmad-method install
    # Select "Install expansion pack" option
    ```
4.  **Use in Your Workflow**: Installed packs integrate seamlessly with existing agents

### Creating Custom Expansion Packs

Use the **expansion-creator** pack to build your own:

1.  [cite\_start]**Define Domain**: What expertise are you capturing? [cite: 166]
2.  [cite\_start]**Design Agents**: Create specialized roles with clear boundaries [cite: 166]
3.  [cite\_start]**Build Resources**: Tasks, templates, checklists for your domain [cite: 166]
4.  [cite\_start]**Test & Share**: Validate with real use cases, share with community [cite: 166]

[cite\_start]**Key Principle**: Expansion packs democratize expertise by making specialized knowledge accessible through AI agents. [cite: 167]

## Getting Help

  * [cite\_start]**Commands**: Use `/help` in any environment to see available commands [cite: 167]
  * [cite\_start]**Agent Switching**: Use `/switch agent-name` with orchestrator for role changes [cite: 167]
  * [cite\_start]**Documentation**: Check `docs/` folder for project-specific context [cite: 167]
  * [cite\_start]**Community**: Discord and GitHub resources available for support [cite: 167]
  * [cite\_start]**Contributing**: See `CONTRIBUTING.md` for full guidelines [cite: 167]