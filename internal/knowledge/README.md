# Knowledge Base Directory

This directory contains the knowledge base files used by the BMAD Discord bot.

## Files

- `bmad.md` - The BMAD-METHOD knowledge base that the bot uses to answer questions

## Purpose

By keeping the knowledge base in the `internal/knowledge` directory, we:
1. Include it directly in the Docker image (no need to mount volumes)
2. Keep it close to the code that uses it
3. Make it part of the application's internal structure

## Updating the Knowledge Base

To update the BMAD knowledge base:
1. Edit the `bmad.md` file with the new content
2. Rebuild the Docker image to include the updated knowledge base
3. Deploy the new image

The bot will load this knowledge base at startup and use it to answer all BMAD-related questions. 