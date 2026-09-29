# Limites de recurso e Travel Mode para Boxes

## Situação

O Travel Mode (`CLAUDE.md`, Omarchy Integration) reduz CPUs só das
Machines: no `Start`, com o host na bateria, a Machine sobe com metade dos
CPUs. Uma Box compila, roda testes e indexa como qualquer processo do host,
sem limite nenhum, e na bateria é justamente ela que mais gasta.

## O que o host oferece

Conferido neste host (2026-09-28):
- cgroups **v2** com gerenciador **systemd**, e o serviço do usuário recebe
  delegação de `cpu`, `memory` e `pids`
  (`/sys/fs/cgroup/user.slice/user-1000.slice/user@1000.service/cgroup.controllers`).
  Sem essa delegação, o Podman rootless não aplica limites e o `crun` falha
  com erro de controlador.
- `podman update --cpus` e `--memory` ajustam **um container já criado**,
  sem recriá-lo. Isso importa porque recriar uma Box perde o que foi
  instalado nela.

## Esboço

- Limites opcionais por Box (CPU e memória) no domínio, como os da Machine
  (`EnvironmentSettings.CPUs`/`MemoryMiB`), aplicados com `podman update`
  (no `Start` e ao salvar nas Settings), sem recriar o container.
- Travel Mode para Boxes: na bateria, `podman update --cpus` com metade dos
  CPUs do host no `Start`, com a mesma regra da Machine (nunca sobrescrever
  um limite que o usuário fixou) e o mesmo opt-out por ambiente.
- `omavm host` passa a dizer se os limites de Box funcionam, lendo os
  controladores delegados; sem delegação, a opção não aparece em vez de
  falhar ao salvar.

## Cuidados

- A Box via Distrobox é criada com `distrobox create`, mas o container é
  do Podman: o `podman update` age por baixo do Distrobox. É preciso
  conferir que um `distrobox enter` depois do update não desfaz o limite.
- Limite de memória baixo mata processos da Box com OOM, o que para quem
  está compilando parece um erro misterioso. A UI deve explicar o limite e
  mostrar o valor em uso.
- Docker como engine tem outro caminho (`docker update`); começar só pelo
  Podman, que é o preferencial no `CLAUDE.md`.

## Fontes

- [How to Use cgroups v2 with Rootless Podman – OneUptime](https://oneuptime.com/blog/post/2026-03-18-use-cgroups-v2-rootless-podman/view)
- [Podman Resource Limits: CPU, Memory, PIDs & I/O](https://www.golinuxcloud.com/podman-resource-limits/)
- [Cannot set --memory on rootless Podman – containers/podman #8330](https://github.com/containers/podman/issues/8330)
