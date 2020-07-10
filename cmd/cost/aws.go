package cost

// awsCmd represents the aws command
//var awsCmd = &cobra.Command{
//	Use:   "aws",
//	Short: "A brief description of your command",
//	Long: `A longer description that spans multiple lines and likely contains examples
//and usage of using your command. For example:
//
//Cobra is a CLI library for Go that empowers applications.
//This application is a tool to generate the needed files
//to quickly create a Cobra application.`,
//	Run: func(cmd *cobra.Command, args []string) {
//		fmt.Println("aws called")
//	},
//}
//
//func init() {
//	awsCmd.AddCommand(getCmd)
//	awsCmd.AddCommand(createCmd)
//	awsCmd.AddCommand(reconcileCmd)
//}

//func initAWSClients() (*organizations.Organizations, *costexplorer.CostExplorer) {
//
//	//Initialize a session with the osd-staging-1 profile or any user that has access to the desired info
//	sess, err := session.NewSessionWithOptions(session.Options{
//		Profile: "osd-staging-1",
//	})
//	if err != nil {
//		log.Fatalln("Unable to generate session:", err)
//	}
//
//	//Create Cost Explorer client
//	ce := costexplorer.New(sess)
//	//Create Organizations client
//	org := organizations.New(sess)
//
//	return org, ce
//}
