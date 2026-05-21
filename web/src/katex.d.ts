declare module 'katex' {
  export interface KatexOptions {
    displayMode?: boolean
    output?: 'html' | 'mathml' | 'htmlAndMathml'
    leqno?: boolean
    fleqn?: boolean
    throwOnError?: boolean
    errorColor?: string
    macros?: Record<string, string>
    minRuleThickness?: number
    colorIsTextColor?: boolean
    maxSize?: number
    maxExpand?: number
    strict?: boolean | string | ((errorCode: string, errorMsg: string, token?: unknown) => boolean | string)
    trust?: boolean | ((context: unknown) => boolean)
    globalGroup?: boolean
  }

  interface KatexInstance {
    render: (expression: string, element: HTMLElement, options?: KatexOptions) => void
    renderToString: (expression: string, options?: KatexOptions) => string
  }

  const katex: KatexInstance
  export default katex
}
