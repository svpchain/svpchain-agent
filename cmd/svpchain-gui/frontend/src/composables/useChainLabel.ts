import {useI18n} from 'vue-i18n'

const CHAIN_LABEL_KEYS: Record<string, string> = {
    'svp-2517-1': 'chain.testnet',
    'svp-2518-1': 'chain.mainnet',
}

export function useChainLabel() {
    const {t} = useI18n()

    function formatChain(id: string): string {
        const key = CHAIN_LABEL_KEYS[id]
        return key ? t(key, {id}) : id
    }

    function chainSelectOptions(ids: readonly string[]) {
        return ids.filter(Boolean).map((value) => ({label: formatChain(value), value}))
    }

    return {formatChain, chainSelectOptions}
}
