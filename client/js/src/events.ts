export interface RepositoryEvent {
  type: 'commit' | 'branch' | 'tag' | 'file' | 'merge';
  repository: string;
  timestamp: string;
  data: any;
}

export type EventHandler = (event: RepositoryEvent) => void;
export type ErrorHandler = (error: Error) => void;

export class EventStream {
  private eventSource: EventSource | null = null;
  private handlers: Set<EventHandler> = new Set();
  private errorHandlers: Set<ErrorHandler> = new Set();
  private url: string;
  private reconnectInterval: number = 5000;
  private maxReconnectAttempts: number = 5;
  private reconnectAttempts: number = 0;

  constructor(url: string) {
    this.url = url;
  }

  connect(): void {
    if (this.eventSource) {
      return;
    }

    this.eventSource = new EventSource(this.url);

    this.eventSource.onmessage = (event) => {
      try {
        const data: RepositoryEvent = JSON.parse(event.data);
        this.handlers.forEach(handler => handler(data));
        this.reconnectAttempts = 0; // Reset on successful message
      } catch (error) {
        this.errorHandlers.forEach(handler => 
          handler(new Error(`Failed to parse event: ${error}`))
        );
      }
    };

    this.eventSource.onerror = (error) => {
      this.errorHandlers.forEach(handler => 
        handler(new Error('EventSource connection error'))
      );

      // Attempt to reconnect
      if (this.reconnectAttempts < this.maxReconnectAttempts) {
        this.reconnectAttempts++;
        setTimeout(() => {
          this.disconnect();
          this.connect();
        }, this.reconnectInterval);
      } else {
        this.disconnect();
        this.errorHandlers.forEach(handler => 
          handler(new Error('Max reconnection attempts reached'))
        );
      }
    };

    this.eventSource.onopen = () => {
      this.reconnectAttempts = 0;
    };
  }

  disconnect(): void {
    if (this.eventSource) {
      this.eventSource.close();
      this.eventSource = null;
    }
  }

  onEvent(handler: EventHandler): () => void {
    this.handlers.add(handler);
    return () => this.handlers.delete(handler);
  }

  onError(handler: ErrorHandler): () => void {
    this.errorHandlers.add(handler);
    return () => this.errorHandlers.delete(handler);
  }

  isConnected(): boolean {
    return this.eventSource?.readyState === EventSource.OPEN;
  }
}

// Observable pattern for TypeScript/RxJS users
export class EventObservable {
  private eventStream: EventStream;

  constructor(url: string) {
    this.eventStream = new EventStream(url);
  }

  subscribe(
    onNext: EventHandler,
    onError?: ErrorHandler,
    onComplete?: () => void
  ): { unsubscribe: () => void } {
    const unsubscribeNext = this.eventStream.onEvent(onNext);
    const unsubscribeError = onError ? this.eventStream.onError(onError) : () => {};
    
    this.eventStream.connect();

    return {
      unsubscribe: () => {
        unsubscribeNext();
        unsubscribeError();
        this.eventStream.disconnect();
        onComplete?.();
      }
    };
  }
}