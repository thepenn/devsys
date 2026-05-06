import React, { useMemo, useRef } from 'react';
import './CodeEditor.less';

// CodeEditor 是一个轻量的 textarea + 高亮层叠加组件, 避免引入 Monaco /
// CodeMirror 这种大依赖. 用于 YAML / Dockerfile 之类的编辑场景.
const escapeHtml = (value = '') =>
  value.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');

const highlightYaml = value => {
  let html = escapeHtml(value);
  html = html.replace(/(#.*?$)/gm, '<span class="code-comment">$1</span>');
  html = html.replace(/^(\s*-?\s*)([A-Za-z0-9_.-]+)(:)/gm, '$1<span class="code-key">$2</span>$3');
  html = html.replace(/(:\s*)([A-Za-z0-9_\-./{}$][^\n]*)/g, (_, prefix, val) => {
    if (/^(&lt;|&amp;)/.test(val)) return `${prefix}${val}`;
    if (/^<span/.test(val)) return `${prefix}${val}`;
    return `${prefix}<span class="code-value">${val}</span>`;
  });
  return html;
};

const highlightDockerfile = value => {
  let html = escapeHtml(value);
  html = html.replace(
    /^(\s*)(FROM|RUN|CMD|COPY|ADD|ENV|ARG|WORKDIR|ENTRYPOINT|EXPOSE|VOLUME|USER|LABEL|ONBUILD|STOPSIGNAL|HEALTHCHECK)(\b)/gim,
    '$1<span class="code-keyword">$2</span>$3'
  );
  html = html.replace(/(#.*?$)/gm, '<span class="code-comment">$1</span>');
  return html;
};

const CodeEditor = ({
  value = '',
  onChange,
  language = 'yaml',
  placeholder = '开始编辑...',
  readOnly = false,
  className = ''
}: {
  value?: string;
  onChange?: (next: string) => void;
  language?: string;
  placeholder?: string;
  readOnly?: boolean;
  className?: string;
}) => {
  const textRef = useRef(null);
  const highlightRef = useRef(null);
  const highlighted = useMemo(
    () => (language === 'dockerfile' ? highlightDockerfile(value) : highlightYaml(value)),
    [language, value]
  );

  const syncScroll = event => {
    if (!highlightRef.current) return;
    highlightRef.current.scrollTop = event.target.scrollTop;
    highlightRef.current.scrollLeft = event.target.scrollLeft;
  };

  return (
    <div className={['code-editor', className].filter(Boolean).join(' ')}>
      <pre className="code-editor__highlight" ref={highlightRef} aria-hidden="true">
        <code dangerouslySetInnerHTML={{ __html: highlighted || escapeHtml(placeholder) }} />
      </pre>
      <textarea
        ref={textRef}
        className="code-editor__textarea"
        value={value}
        onChange={e => onChange?.(e.target.value)}
        onScroll={syncScroll}
        spellCheck={false}
        placeholder={placeholder}
        readOnly={readOnly}
      />
    </div>
  );
};

export default CodeEditor;
