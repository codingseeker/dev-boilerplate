export class DemoService {
  constructor(logger) {
    this.logger = logger;
  }

  async getMessage() {
    this.logger.debug('DemoService.getMessage called');
    return {
      message: 'Hello from js-starter boilerplate',
      timestamp: new Date().toISOString(),
    };
  }
}

export function createDemoService(logger) {
  return new DemoService(logger);
}
