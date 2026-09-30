# CLI para agentes: códigos de saída e erros em JSON

## Situação

O `CLAUDE.md` quer a OmaVM "controlável por humanos e por agentes" e já
trata o JSON do `list`/`status --json` como contrato estável (a barra do
Omarchy consome). Mas os **erros** não têm contrato:

- toda falha sai com código **1**, seja entrada inválida, ambiente
  inexistente, recurso indisponível para aquele tipo ou pane real;
- a mensagem vai só como texto para o stderr, em inglês, mesmo com
  `--json`.

Um agente precisa ler texto para saber se deve corrigir o pedido, criar o
ambiente ou desistir. Desde 2026-09-28 a Core já classifica os erros
(`ErrInvalidInput`, `ErrNotFound`, `ErrAlreadyExists`, `ErrUnsupported`,
com todas as validações cobertas por
`TestValidationErrorsAreInvalidInput`), então falta só expor isso.

## Esboço

Códigos de saída estáveis, no espírito do `sysexits.h`, mas poucos:

| Classe | Código | Significa para quem chama |
|---|---|---|
| sucesso | 0 | — |
| erro geral | 1 | falha do sistema ou do backend; talvez tentar de novo |
| uso/entrada inválida (`ErrInvalidInput`, flags) | 2 | corrigir o pedido |
| não encontrado (`ErrNotFound`) | 3 | o ambiente não existe |
| já existe (`ErrAlreadyExists`) | 4 | escolher outro nome |
| indisponível para este tipo (`ErrUnsupported`) | 5 | não adianta tentar de novo |

Com `--json`, a falha também sai como JSON no stdout:
`{"error": {"code": "not_found", "message": "environment not found: x"}}`,
com `code` estável em snake_case e `message` para gente.

## Cuidados

- É uma mudança de contrato: scripts que tratam "qualquer não-zero" continuam
  funcionando, mas quem compara com `1` precisa saber. Documentar no README
  e na ajuda da CLI antes de publicar.
- A GUI hoje só lê stderr; ela pode passar a usar o `code` para decidir
  texto e ações (por exemplo, "criar ambiente" ao receber `not_found`).

## Fontes

- [Designing a CLI for AI agents – Arcjet](https://blog.arcjet.com/designing-a-cli-for-ai-agents/)
- [Writing CLI Tools That AI Agents Actually Want to Use – DEV](https://dev.to/uenyioha/writing-cli-tools-that-ai-agents-actually-want-to-use-39no)
- [A compact contract for CLIs for agents and development tools](https://gist.github.com/peterc/bef25ee122d233c53a851b9ace29efef)
