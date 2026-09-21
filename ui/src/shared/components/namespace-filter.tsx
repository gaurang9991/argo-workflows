import {Select} from 'argo-ui/src/components/select/select';
import * as React from 'react';

import * as nsUtils from '../namespaces';
import {InputFilter} from './input-filter';

export const NamespaceFilter = (props: {value: string; onChange: (namespace: string) => void; extraNamespaces?: string[]}) => {
    const managedNamespace = nsUtils.getManagedNamespace();
    const managedNamespaces = nsUtils.getManagedNamespaces();
    if (managedNamespace) {
        return <>{managedNamespace}</>;
    }
    if (managedNamespaces.length === 1) {
        return <>{managedNamespaces[0]}</>;
    }
    if (managedNamespaces.length > 0) {
        return <Select options={managedNamespaces} value={props.value} onChange={option => props.onChange(option.value)} />;
    }
    return <InputFilter value={props.value} name='ns' onChange={ns => props.onChange(ns)} extraSuggestions={props.extraNamespaces} filterSuggestions />;
};
