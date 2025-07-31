# Technical Assumptions

* **Repository Structure**: A single **Polyrepo** will be used for the bot's codebase.
* **Service Architecture**: We will proceed with a **Monolithic service** for the MVP.
* **Testing requirements**: The project requires **Unit Tests** for all core logic and **Integration Tests** for the connection to the Discord API and the Gemini CLI wrapper.
* **Additional Technical Assumptions and Requests**:
    * The backend service must be written in **Golang**.
    * The application must be containerized with **Docker** for deployment.

## Implementation Decisions

* **Knowledge Base Storage**: The BMAD knowledge base (`bmadprompt.md`) is stored in `internal/knowledge/bmad.md` and included directly in the Docker image. This eliminates the need to mount external volumes and ensures the knowledge base is always available with the deployed application.