# Logs no journal do systemd

## Por que pensar nisso

Em 2026-09-28 a OmaVM ganhou rotação própria de log (5 MiB, um `.1`) porque
o arquivo crescia sem limite. Mas o Omarchy roda systemd, e o **journald**
já faz exatamente isso, melhor: limite de tamanho global, compressão,
consulta por campo e por tempo (`journalctl`), e visualização junto com o
resto do sistema. É o caso típico de "integre, não reimplemente" (Backend
Rules), aplicado à observabilidade.

## Como seria

- O journal aceita entradas pelo **protocolo nativo**: datagramas num
  socket `AF_UNIX`/`SOCK_DGRAM` em `/run/systemd/journal/socket`, cada um
  com pares `CAMPO=valor`. Dá para falar isso só com a biblioteca padrão do
  Go, sem cgo e sem dependência nova (o projeto não tem `go.sum`); há
  inclusive um handler `slog` oficial do projeto systemd
  (`github.com/systemd/slog-journal`) como referência de implementação.
- Campos estruturados do `slog` viram campos do journal (`OMAVM_CMD`,
  `OMAVM_ENV`, …), e `SYSLOG_IDENTIFIER=omavm` permite
  `journalctl --user -t omavm` ou `journalctl -t omavm -g create`.
- Sem journal (outra init, container), cai no arquivo atual com rotação:
  o comportamento de hoje continua sendo o fallback.

## Cuidados

- `/var/log/omavm` e o README documentam o arquivo como destino; mudar o
  padrão é mudança visível que precisa de nota no README e no `CLAUDE.md`.
- Comandos de leitura continuam fora do log (nível Debug), senão o journal
  enche com poll do mesmo jeito.
- Mensagens grandes: o datagrama tem limite; o protocolo nativo prevê
  mandar o conteúdo num memfd selado quando passa do tamanho.

## Fontes

- [Native Journal Protocol – systemd.io](https://systemd.io/JOURNAL_NATIVE_PROTOCOL/)
- [slogjournal – github.com/systemd/slog-journal](https://pkg.go.dev/github.com/systemd/slog-journal)
- [go-systemd journal (pure Go)](https://pkg.go.dev/github.com/coreos/go-systemd/v22/journal)
