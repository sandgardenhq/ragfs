# ragfs

A filesystem interface for LLM agents that maps tool calls and data reading operations into intuitive file system structures.

## Concept

ragfs leverages the fact that LLM models are well-trained on understanding and manipulating file systems. By mapping agentic operations (tool calls, data retrieval, context management) to filesystem primitives, we create an interface that LLMs can naturally work with.

## Goals

- Map tool calls to filesystem operations
- Provide intuitive data reading through file-like interfaces
- Enable LLM agents to leverage their strong filesystem manipulation capabilities
- Create a natural bridge between agent reasoning and tool execution

## Status

Project initialization - under development
