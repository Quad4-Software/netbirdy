export interface RelayStatus {
  enabled: boolean;
  address: string;
  transports: string[];
  connected_peers: number;
}
