import { readFile } from 'node:fs/promises';
import { Command } from 'commander';
import pc from 'picocolors';
import { EnumStatusCode } from '@wundergraph/cosmo-connect/dist/common_pb';
import { splitLabel } from '@wundergraph/cosmo-shared';
import { join } from 'pathe';
import { BaseCommandOptions } from '../../../core/types/types.js';
import { baseHeaders } from '../../../core/config.js';

export default (opts: BaseCommandOptions) => {
  const schemaPush = new Command('create');
  schemaPush.description('Creates a federated subgraph on the control plane.');
  schemaPush.argument(
    '<name>',
    'The name of the subgraph to create. It is usually in the format of <org>.<service.name> and is used to uniquely identify your subgraph.',
  );
  schemaPush.requiredOption(
    '-r, --routing-url <url>',
    'The routing url of your subgraph. This is the url that the subgraph will be accessible at.',
  );
  schemaPush.requiredOption(
    '--label [labels...]',
    'The labels to apply to the subgraph. The labels are passed in the format <key>=<value> <key>=<value>.',
  );
  schemaPush.option(
    '--header [headers...]',
    'The headers to apply when the subgraph is introspected. This is used for authentication and authorization.',
  );
  schemaPush.action(async (name, options) => {
    const resp = await opts.client.platform.createFederatedSubgraph(
      {
        name,
        labels: options.label.map((label: string) => {
          const { key, value } = splitLabel(label);
          return {
            key,
            value,
          };
        }),
        routingUrl: options.routingUrl,
        headers: options.header,
      },
      {
        headers: baseHeaders,
      },
    );

    if (resp.response?.code === EnumStatusCode.OK) {
      console.log(pc.dim(pc.green(`A new subgraph called '${name}' was created.`)));
    } else {
      console.log(`Failed to create subgraph '${pc.bold(name)}'.`);
      if (resp.response?.details) {
        console.log(pc.red(pc.bold(resp.response?.details)));
      }
      process.exit(1);
    }
  });

  return schemaPush;
};
