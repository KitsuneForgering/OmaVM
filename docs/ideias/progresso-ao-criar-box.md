# Progresso ao criar uma Box

## Situação

Criar uma Box de uma imagem que ainda não está no host baixa centenas de
MB dentro do `distrobox create`. Durante esse tempo:
- a GUI mostra só "Working: creating…", por minutos, sem sinal de avanço;
- o registro (`environments.json`) fica travado durante todo o
  `backend.Create`, então outro `create`, `rm`, `settings` ou `snapshot`
  espera junto. Desde 2026-09-28 a CLI pelo menos avisa no terminal
  ("waiting for another OmaVM operation to finish…").

## O que o Podman oferece

- `podman pull` mostra barra de progresso no terminal, mas só para humanos.
- A API REST (`/libpod/images/pull` no socket rootless
  `$XDG_RUNTIME_DIR/podman/podman.sock`) devolve um fluxo de JSON com
  **etapas** ("Trying to pull", "Copying blob", "Writing manifest"), mas
  não bytes baixados: o pedido para ter isso
  ([containers/podman#24887](https://github.com/containers/podman/issues/24887))
  continua aberto.

## Esboço

1. Separar o pull do create: `podman pull <imagem>` **antes** de travar o
   registro e só então `distrobox create` (que acha a imagem local e não
   baixa de novo). O registro fica travado segundos, não minutos, e uma
   falha de rede aparece como falha de download, não como falha genérica
   do create.
2. Repassar as etapas do pull como status: a CLI imprime as linhas no
   terminal; a GUI mostra "Baixando Fedora (etapa 2 de 4)…" a partir do
   fluxo JSON da API, sem inventar porcentagem que o Podman não fornece.
3. Checar antes se a imagem já existe (`podman image exists`) para não
   mostrar "baixando" à toa.

O pull fica no adapter do Distrobox/Podman (Backend Rules); a Core só vê
"preparando" e "criando".

## Fontes

- [podman-pull — Podman documentation](https://docs.podman.io/en/latest/markdown/podman-pull.1.html)
- [How to Use the Podman REST API to Pull Images – OneUptime](https://oneuptime.com/blog/post/2026-03-18-use-podman-rest-api-pull-images/view)
- [Implement Progress Details in podman pull API – containers/podman#24887](https://github.com/containers/podman/issues/24887)
- [No way to obtain image pull progress – containers/podman#12341](https://github.com/containers/podman/issues/12341)
