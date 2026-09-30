# Aceleração de GPU por native context

## O que é

Além do virgl (OpenGL traduzido) e do Venus (Vulkan serializado), a
virtio-gpu do QEMU agora suporta **DRM native context**: o guest usa o
driver Mesa nativo da GPU do host, e a virtio-gpu só medeia a UAPI do
kernel. Isso gasta menos CPU que virgl e Venus, e o guest ganha OpenGL,
Vulkan e decodificação de vídeo (VA-API) do próprio driver nativo.

## Situação em 2026

- QEMU: a propriedade `drm_native_context` existe no `virtio-vga-gl` (o
  QEMU 11.1.1 deste host já a lista), exigindo `blob=on` e `hostmem`.
- virglrenderer ≥ 1.0 suporta o protocolo; o i915 pede ≥ 1.3.0 e kernel
  6.13+ no host, com Mesa recente e kernel 6.14+ no guest.
- AMD e Qualcomm (msm) estão upstream. Intel i915 tem merge requests
  abertos; **Intel Xe (GPUs novas) ainda não está upstream** em
  virglrenderer nem Mesa, e ninguém assumiu esse trabalho.

## Encaixe na OmaVM

É o próximo passo natural depois do Venus (ligado por padrão desde
2026-09-28): mesma detecção estática do host (`omavm host`), mesma opção
por Machine. Mas hoje só vale para AMD. No host de desenvolvimento
(Intel Tiger Lake, i915) depende de patches ainda não publicados.

## Esboço

- Estender `detectGraphics` com `nativeContext` para AMD (`amdgpu`), com o
  mesmo tipo de checagem estática do Venus (versão do virglrenderer,
  kernel).
- Preferir native context ao Venus quando os dois estiverem disponíveis;
  uma Machine usa um ou outro.
- Não oferecer para Intel até o suporte ser upstream, mesmo existindo
  scripts de terceiros que habilitam o Xe.

## Fontes

- [VirtIO GPU — QEMU documentation](https://www.qemu.org/docs/master/system/devices/virtio/virtio-gpu.html)
- [Support virtio-gpu DRM native context (patch series, v6)](https://patchew.org/QEMU/20250126201121.470990-1-dmitry.osipenko@collabora.com/)
- [xe-native-context-enablement](https://github.com/cmspam/xe-native-context-enablement)
