import { h } from 'vue'
import { useI18n } from 'vue-i18n'
import { NButton } from 'naive-ui'
import * as App from '../../wailsjs/go/desktop/App'

export function useAddressCell(onStatus: (msg: string) => void) {
  const { t } = useI18n()

  function copyAddress(addr: string) {
    if (!addr) return
    App.CopyText(addr)
    onStatus(t('status.addressCopied'))
  }

  function addressCell(addr: string) {
    return h('div', { class: 'addr-cell' }, [
      h('span', { class: 'addr-text', title: addr }, addr),
      h(
        NButton,
        { size: 'tiny', quaternary: true, onClick: () => copyAddress(addr) },
        { default: () => t('btn.copyShort') },
      ),
    ])
  }

  return { addressCell }
}
