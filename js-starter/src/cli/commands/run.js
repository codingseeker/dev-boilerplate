export async function runCommand(config, logger, services) {
  logger.info(
    { config: { appName: config.appName, nodeEnv: config.nodeEnv } },
    'Running core application'
  );
  const result = await services.demoService.getMessage();
  logger.info(result, 'Core application result');
  return result;
}
