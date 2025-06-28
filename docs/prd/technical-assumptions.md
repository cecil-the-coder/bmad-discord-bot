# Technical Assumptions

* **Repository Structure**: A single **Polyrepo** will be used for the bot's codebase.
* **Service Architecture**: We will proceed with a **Monolithic service** for the MVP.
* **Testing requirements**: The project requires **Unit Tests** for all core logic and **Integration Tests** for the connection to the Discord API and the Gemini CLI wrapper.
* **Additional Technical Assumptions and Requests**:
    * The backend service must be written in **Golang**.
    * The application must be containerized with **Docker** for deployment.