# Security

  * **Secrets Management**: The Discord Bot Token and any Google Cloud/Gemini API keys will be managed via environment variables, passed securely to the Docker container at runtime. They will never be checked into source control.
  * **Input Sanitization**: While the risk is low, any user-provided text that is used to construct a command-line argument for the Gemini CLI will be properly escaped to prevent potential command injection vulnerabilities.