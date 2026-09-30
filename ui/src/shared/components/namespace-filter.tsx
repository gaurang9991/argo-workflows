import * as React from 'react';

import * as nsUtils from '../namespaces';
import {InputFilter} from './input-filter';

export const NamespaceFilter = (props: {value: string; onChange: (namespace: string) => void; extraNamespaces?: string[]}) => {
    const managedNamespaces = nsUtils.getManagedNamespaces();
    if (managedNamespaces.length > 1) {
        // a static allowlist of several namespaces: restrict the selector to that set
        return (
            <select value={props.value} onChange={e => props.onChange(e.target.value)}>
                {managedNamespaces.map(ns => (
                    <option key={ns} value={ns}>
                        {ns}
                    </option>
                ))}
            </select>
        );
    }
    return nsUtils.getManagedNamespace() ? (
        <>{nsUtils.getManagedNamespace()}</>
    ) : (
        <InputFilter value={props.value} name='ns' onChange={ns => props.onChange(ns)} extraSuggestions={props.extraNamespaces} filterSuggestions />
    );
};
