# Goals and Background Context

## Goals

* Increase the adoption and effective, consistent use of the BMAD-METHOD framework.
* Allow users to get accurate, helpful answers to their questions in under 30 seconds.
* Provide a real-time visual indicator of the bot's API rate-limit status via its Discord status.
* Enable conversational follow-up questions within Discord threads.

## Background Context

The core problem this project addresses is the inefficiency developers face when searching through large BMAD documentation files. This manual process disrupts workflow and can lead to inconsistent application of the framework.

The proposed solution is a Golang-based Discord bot that acts as a specialized, on-demand expert. It will use the Gemini CLI to query the BMAD knowledge base and provide synthesized, conversational answers within Discord threads, allowing for context-aware follow-up questions. The entire application will be containerized for portable deployment.

## Change Log

| Date | Version | Description | Author |
| :--- | :--- | :--- | :--- |
| 2025-06-28 | 1.0 | Initial PRD creation | John, PM |