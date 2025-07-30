export { GovcClient, GovcError } from './client';
export { RepositoryClient } from './repository';
export { TransactionClient } from './transaction';
export { AuthClient } from './auth';
export { 
  EventStream, 
  EventObservable, 
  RepositoryEvent,
  EventHandler,
  ErrorHandler 
} from './events';

// Export all types
export * from './types';

// Default export for convenience
import { GovcClient } from './client';
export default GovcClient;