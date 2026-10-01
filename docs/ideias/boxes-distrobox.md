# Boxes: Distrobox v2 e ações que faltam

## Risco: Distrobox v2 quebra a exportação de apps

O Distrobox v2 (reescrito em Go, em release candidate desde 2026) mantém os
argumentos de `create`/`enter`/`stop`/`rm`/`list`, mas vira um binário
único: **`distrobox-export` deixa de existir como executável separado**
(passa a ser `distrobox export`). Apps e binários exportados por Boxes do
v1 precisam que a Box seja recriada.

Impacto na OmaVM: `internal/backend/box/distrobox/apps.go` executa
`distrobox enter --name <box> -- distrobox-export --app <id>` (e
`--delete`). O v1 instalado aqui (1.8.2.5) não tem subcomando `export` no
host; o export roda dentro da Box. O anúncio do v2 não diz como o export é
chamado de dentro do container.

Plano, quando houver necessidade real (v2 estável ou um usuário no RC):
1. Testar o adapter contra o `v2.0.0-rc` num ambiente isolado, sem
   substituir o Distrobox do host.
2. Descobrir a forma certa de exportar no v2 e detectar a versão
   (`distrobox version`), em vez de tentar os dois comandos às cegas.
3. Avisar na UI quando apps exportadas no v1 precisarem que a Box seja
   recriada, em vez de falhar em silêncio.

Não implementar antes disso: seria código especulativo para uma versão
ainda não estável (Coding Principles).

## Ações de Box que outras GUIs oferecem

BoxBuddy e DistroShelf (GUIs GTK4 para Distrobox) oferecem por Box: abrir
terminal, **atualizar** (`distrobox upgrade`), **clonar**
(`distrobox create --clone`), ver e exportar apps, parar e apagar, e uma
linha de uso de CPU e memória.

Para a OmaVM:
- **Clone** já está na lista de ações do Experience Center (UX Principle
  #5) e ainda não existe para nenhum dos dois tipos. Para Box é integração
  direta com o `--clone` nativo; o custo (cópia da imagem do container)
  precisa aparecer na UI (UX Principle #10).
- **Atualizar** a Box (`distrobox upgrade`) cabe no menu do card; é longa,
  então precisa do mesmo tratamento de ação em andamento que Restart já tem
  (inclusive sobreviver ao fechamento da janela, corrigido em 2026-09-27).
- **Uso de CPU e memória**: o `podman stats --no-stream --format json` dá o
  número sem daemon. No Experience Center, só com a Box rodando e sem
  polling pesado (o poll de 3 s já foi um problema de CPU).

## Fontes

- [Announcing the next generation of Distrobox](https://distrobox.it/posts/announcing_distrobox_next/)
- [Distrobox changelog](https://whatsnew.fyi/product/distrobox)
- [DistroShelf](https://github.com/ranfdev/DistroShelf)
- [BoxBuddy](https://github.com/Dvlv/BoxBuddyRS)
- [BoxBuddy vs DistroShelf – Linux Adictos](https://en.linuxadictos.com/BoxBuddy-vs.-DistroShelf:-A-complete-comparison-and-practical-guide-between-these-two-DistroBox-managers..html)
