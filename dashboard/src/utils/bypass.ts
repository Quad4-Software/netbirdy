// EDR peer-bypass is a cloud-only feature; the hooks below preserve the call
// shapes used by peer UI while returning inert values.
export const isBypassed = (_id?: string): boolean => false;
export const canBypass = (): boolean => false;
export const bypassCompliance = (_id?: string): Promise<void> => Promise.resolve();
export const revokeBypass = (_id?: string): Promise<void> => Promise.resolve();
export const bypassedPeerIds = new Set<string>();
