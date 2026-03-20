package backup

import "github.com/spf13/pflag"

// backupFlags holds the parsed command-line flag values for the backup command.
type backupFlags struct {
	clusterID string
	reason    string
	// labels holds optional key=value pairs that are forwarded to the Velero
	// backup CR via --labels. Populated by repeated --label flags.
	labels map[string]string
	// annotations holds optional key=value pairs that are forwarded to the
	// Velero backup CR via --annotations. Populated by repeated --annotation flags.
	annotations map[string]string
}

// AddFlags binds the command-line flags for this command to the given FlagSet.
func (f *backupFlags) AddFlags(flags *pflag.FlagSet) {
	flags.StringVarP(&f.clusterID, "cluster-id", "C", "", "Internal ID, name, or external ID of the HCP cluster")
	flags.StringVar(&f.reason, "reason", "", "Reason for privilege elevation (e.g., OHSS-1234 or PD incident ID)")
	flags.StringToStringVar(&f.labels, "label", nil, "Label to add to the Velero Backup CR (key=value); may be repeated")
	flags.StringToStringVar(&f.annotations, "annotation", nil, "Annotation to add to the Velero Backup CR (key=value); may be repeated")
}
