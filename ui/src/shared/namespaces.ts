const managedNamespaceKey = 'managedNamespace';
const managedNamespacesKey = 'managedNamespaces';
const userNamespaceKey = 'userNamespace';
const currentNamespaceKey = 'current_namespace';

// just a temp function, this gets set in app-router
let onNamespaceChange = (x: string) => x;
export function setOnNamespaceChange(f: any) {
    onNamespaceChange = f;
}

function fixLocalStorageString(x: string): string {
    // empty string is valid, so we cannot use `truthy`
    if (x == null || x == 'null' || x == 'undefined') {
        return undefined; // explicitly return undefined
    }
    return x;
}

export function setUserNamespace(value: string) {
    if (value) {
        localStorage.setItem(userNamespaceKey, value);
    } else {
        localStorage.removeItem(userNamespaceKey);
    }
}

function getUserNamespace() {
    return fixLocalStorageString(localStorage.getItem(userNamespaceKey));
}

export function setManagedNamespace(value: string) {
    if (value) {
        localStorage.setItem(managedNamespaceKey, value);
    } else {
        localStorage.removeItem(managedNamespaceKey);
    }
}

export function getManagedNamespace() {
    return fixLocalStorageString(localStorage.getItem(managedNamespaceKey));
}

export function setManagedNamespaces(value: string[]) {
    if (value?.length) {
        localStorage.setItem(managedNamespacesKey, JSON.stringify(value));
    } else {
        localStorage.removeItem(managedNamespacesKey);
    }
}

export function getManagedNamespaces(): string[] {
    const value = localStorage.getItem(managedNamespacesKey);
    return value ? JSON.parse(value) : [];
}

export function setCurrentNamespace(value: string) {
    if (value != null) {
        localStorage.setItem(currentNamespaceKey, value);
    } else {
        localStorage.removeItem(currentNamespaceKey);
    }
    onNamespaceChange(getCurrentNamespace());
}

export function getCurrentNamespace() {
    const currentNamespace = fixLocalStorageString(localStorage.getItem(currentNamespaceKey));
    const managedNamespaces = getManagedNamespaces();
    if (managedNamespaces.length > 0) {
        return currentNamespace && managedNamespaces.includes(currentNamespace) ? currentNamespace : managedNamespaces[0];
    }
    return currentNamespace ?? (getUserNamespace() || getManagedNamespace());
}

// return a namespace, favoring managed namespace when set
export function getNamespace(namespace: string) {
    return getManagedNamespace() || namespace;
}

// return a namespace, never return null/undefined/empty string, default to "default"
export function getNamespaceWithDefault(namespace: string) {
    return namespace || getCurrentNamespace() || getUserNamespace() || getManagedNamespace() || 'default';
}

// extract the unique, sorted set of namespaces present on a list of namespaced k8s objects
export function getUniqueNamespaces<T extends {metadata?: {namespace?: string}}>(items: T[] | null | undefined): string[] {
    if (!items) {
        return [];
    }
    const set = new Set<string>();
    for (const item of items) {
        if (item.metadata?.namespace) {
            set.add(item.metadata.namespace);
        }
    }
    return Array.from(set).sort();
}
